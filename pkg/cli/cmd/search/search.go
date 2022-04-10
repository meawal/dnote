/* Copyright (C) 2019, 2020, 2021 Monomax Software Pty Ltd
 *
 * This file is part of Dnote.
 *
 * Dnote is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Dnote is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with Dnote.  If not, see <https://www.gnu.org/licenses/>.
 */

package search

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/dnote/dnote/pkg/cli/context"
	"github.com/dnote/dnote/pkg/cli/infra"
	"github.com/dnote/dnote/pkg/cli/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var example = `
	# search notes for an expression
	dnote search rpoplpush

	# search notes for an expression with multiple words
	dnote search "building a heap"

	# search notes within a book
	dnote search "merge sort" -b algorithm
	`

var bookName string
var all bool

func preRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("Incorrect number of argument")
	}

	return nil
}

// NewCmd returns a new remove command
func NewCmd(ctx context.DnoteCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Search notes extensively for matching expression",
		Aliases: []string{"s"},
		Example: example,
		PreRunE: preRun,
		RunE:    newRun(ctx),
	}

	f := cmd.Flags()
	f.StringVarP(&bookName, "book", "b", "", "book name to find notes in")
	f.BoolVarP(&all, "all", "a", false, "search all notes including the archived")
	
	return cmd
}

// noteInfo is an information about the note to be printed on screen
type noteInfo struct {
	RowID     int
	BookLabel string
	Body      string
	Archive   bool
}

// formatFTSSnippet turns the matched snippet from a full text search
// into a format suitable for CLI output
func formatFTSSnippet(s string) (string, error) {
	// first, strip all new lines
	body := newLineReg.ReplaceAllString(s, " ")

	var format, buf strings.Builder
	var args []interface{}

	toks := tokenize(body)

	for _, tok := range toks {
		if tok.Kind == tokenKindHLBegin || tok.Kind == tokenKindEOL {
			format.WriteString("%s")
			args = append(args, buf.String())

			buf.Reset()
		} else if tok.Kind == tokenKindHLEnd {
			format.WriteString("%s")
			str := log.ColorYellow.Sprintf("%s", buf.String())
			args = append(args, str)

			buf.Reset()
		} else {
			if err := buf.WriteByte(tok.Value); err != nil {
				return "", errors.Wrap(err, "building string")
			}
		}
	}

	return fmt.Sprintf(format.String(), args...), nil
}

// escapePhrase escapes the user-supplied FTS keywords by wrapping each term around
// double quotations so that they are treated as 'strings' as defined by SQLite FTS5.
func escapePhrase(s string) (string, error) {
	var b strings.Builder

	terms := strings.Fields(s)

	for idx, term := range terms {
		if _, err := b.WriteString(fmt.Sprintf("\"%s\"", term)); err != nil {
			return "", errors.Wrap(err, "writing string to builder")
		}

		if idx != len(term)-1 {
			if err := b.WriteByte(' '); err != nil {
				return "", errors.Wrap(err, "writing space to builder")
			}
		}
	}

	return b.String(), nil
}

func doQuery(ctx context.DnoteCtx, query, bookName string, all bool) (*sql.Rows, error) {
	db := ctx.DB

	sql := `SELECT
		notes.rowid,
		books.label AS book_label,
		note_fts.body,
		books.archive as archive
	FROM note_fts
	INNER JOIN notes ON notes.rowid = note_fts.rowid
	INNER JOIN books ON notes.book_uuid = books.uuid
	WHERE note_fts.body LIKE ?`
	args := []interface{}{query}

	if bookName != "" {
		sql = fmt.Sprintf("%s AND books.label LIKE ?", sql)
		args = append(args, bookName)
	} else if !all {
		sql = fmt.Sprintf("%s AND books.archive = false", sql)
	}

	rows, err := db.Query(sql, args...)

	return rows, err
}

func indexAt(s, key string, n int) int {
	idx := strings.Index(s[n:], key)
	if idx > -1 {
		idx += n
	}
	return idx
}

func newRun(ctx context.DnoteCtx) infra.RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		phrase := "%" + strings.Join(args[:], "%") + "%"

		rows, err := doQuery(ctx, phrase, bookName, all)
		if err != nil {
			return errors.Wrap(err, "querying notes")
		}
		defer rows.Close()

		infos := []noteInfo{}
		for rows.Next() {
			var info noteInfo

			var body string
			err = rows.Scan(&info.RowID, &info.BookLabel, &body, &info.Archive)
			
			c := 60
			var phrase_lwr = strings.ToLower(args[0])
			var s = 0
			var e = 0
			var idx = strings.Index(strings.ToLower(body), phrase_lwr)
			for idx > -1 {
				s = idx - c
				if s > e {
					body = body[:e] + "<dnotehl>...</dnotehl>" + body[s:idx] + 
						"<dnotehl>" + body[idx:idx+len(phrase_lwr)] + "</dnotehl>" +
						body[idx+len(phrase_lwr):]
					idx = idx - (s-e) + 22 + 9
					
				} else {
					body = body[:idx] + 
						"<dnotehl>" + body[idx:idx+len(phrase_lwr)] + "</dnotehl>" + 
						body[idx+len(phrase_lwr):]
					idx = idx + 9
				}
				e = idx + len(phrase_lwr) + 10 + c
				idx = indexAt(strings.ToLower(body), phrase_lwr, idx+len(phrase_lwr)+10)
			}
			if (e != 0) && (e < len(body)) {
				body = body[:e] + "<dnotehl>...</dnotehl>"
			}
			
			if err != nil {
				return errors.Wrap(err, "scanning a row")
			}

			body, err := formatFTSSnippet(body)
			if err != nil {
				return errors.Wrap(err, "formatting a body")
			}

			info.Body = body

			infos = append(infos, info)
		}

		for _, info := range infos {
			var bookLabel string
			if info.Archive {
				bookLabel = log.ColorGray.Sprintf("(%s)", info.BookLabel)
			} else {
				bookLabel = log.ColorYellow.Sprintf("(%s)", info.BookLabel)
			}
			
			rowid := log.ColorYellow.Sprintf("(%d)", info.RowID)

			log.Plainf("%s %s %s\n", bookLabel, rowid, info.Body)
		}
		
		return nil
	}
}
