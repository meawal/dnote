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

package ls

import (
	"database/sql"
	"fmt"
	"strings"
	"strconv"

	"github.com/dnote/dnote/pkg/cli/context"
	"github.com/dnote/dnote/pkg/cli/infra"
	"github.com/dnote/dnote/pkg/cli/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var example = `
 * List all books
 dnote ls

 * List notes in a book
 dnote ls javascript
 `

var deprecationWarning = `and "view" will replace it in the future version.

Run "dnote view --help" for more information.
`

func preRun(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return errors.New("Incorrect number of argument")
	}

	return nil
}

// NewCmd returns a new ls command
func NewCmd(ctx context.DnoteCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "ls <book name?>",
		Aliases:    []string{"l", "notes"},
		Short:      "List all notes",
		Example:    example,
		RunE:       NewRun(ctx, false),
		PreRunE:    preRun,
		Deprecated: deprecationWarning,
	}

	return cmd
}

// NewRun returns a new run function for ls
func NewRun(ctx context.DnoteCtx, all bool) infra.RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			if err := printBooks(ctx, all); err != nil {
				return errors.Wrap(err, "viewing books")
			}

			return nil
		}

		bookName := args[0]
		if strings.Contains(bookName, "%") {
			if err := printMatchBooks(ctx, bookName, false); err != nil {
				return errors.Wrap(err, "viewing books")
			}

			return nil
		}

		if err := PrintNotes(ctx, bookName); err != nil {
			return errors.Wrapf(err, "viewing book '%s'", bookName)
		}

		return nil
	}
}

// bookInfo is an information about the book to be printed on screen
type bookInfo struct {
	BookLabel string
	NoteCount int
	Archive bool
}

// noteInfo is an information about the note to be printed on screen
type noteInfo struct {
	RowID int
	Body  string
}

type noteID struct {
	RowID int
}

// getNewlineIdx returns the index of newline character in a string
func getNewlineIdx(str string) int {
	var ret int

	ret = strings.Index(str, "\n")

	if ret == -1 {
		ret = strings.Index(str, "\r\n")
	}

	return ret
}

// formatBody returns an excerpt of the given raw note content and a boolean
// indicating if the returned string has been excertped
func formatBody(noteBody string) (string, bool) {
	trimmed := strings.TrimRight(noteBody, "\r\n")
	newlineIdx := getNewlineIdx(trimmed)

	if newlineIdx > -1 {
		ret := strings.Trim(trimmed[0:newlineIdx], " ")

		return ret, true
	}

	return strings.Trim(trimmed, " "), false
}

func printBookLine(info bookInfo, nameOnly bool) {
	if nameOnly {
		fmt.Println(info.BookLabel)
	} else if info.Archive == false{
		log.Printf("%s %s\n", info.BookLabel, log.ColorYellow.Sprintf("(%d)", info.NoteCount))
	} else {
		log.Printf("%s %s\n", log.ColorGray.Sprintf("%s",info.BookLabel), log.ColorYellow.Sprintf("(%d)", info.NoteCount))
	}
}

func printBooks(ctx context.DnoteCtx, all bool) error {
	db := ctx.DB

	rows, err := db.Query(`SELECT books.label, books.archive, count(notes.uuid) note_count
	FROM books
	LEFT JOIN notes ON notes.book_uuid = books.uuid AND notes.deleted = false
	WHERE books.deleted = false
		AND books.archive = false
	GROUP BY books.uuid
	ORDER BY books.label ASC;`)
	if err != nil {
		return errors.Wrap(err, "querying books")
	}
	defer rows.Close()

	infos := []bookInfo{}
	for rows.Next() {
		var info bookInfo
		err = rows.Scan(&info.BookLabel, &info.Archive, &info.NoteCount)
		if err != nil {
			return errors.Wrap(err, "scanning a row")
		}

		infos = append(infos, info)
	}

	for _, info := range infos {
		printBookLine(info, false)
	}

	if all {
		rows, err := db.Query(`SELECT books.label, books.archive, count(notes.uuid) note_count
		FROM books
		LEFT JOIN notes ON notes.book_uuid = books.uuid AND notes.deleted = false
		WHERE books.deleted = false
			AND books.archive = true
		GROUP BY books.uuid
		ORDER BY books.label ASC;`)
		if err != nil {
			return errors.Wrap(err, "querying books")
		}
		defer rows.Close()

		infos := []bookInfo{}
		for rows.Next() {
			var info bookInfo
			err = rows.Scan(&info.BookLabel, &info.Archive, &info.NoteCount)
			if err != nil {
				return errors.Wrap(err, "scanning a row")
			}

			infos = append(infos, info)
		}

		for _, info := range infos {
			printBookLine(info, false)
		}
	}
	return nil
}

func printMatchBooks(ctx context.DnoteCtx, keyw string, nameOnly bool) error {
	db := ctx.DB

	rows, err := db.Query(`SELECT books.label, books.archive, count(notes.uuid) note_count
	FROM books
	LEFT JOIN notes ON notes.book_uuid = books.uuid AND notes.deleted = false
	WHERE books.deleted = false
		and books.label LIKE ?
	GROUP BY books.uuid
	ORDER BY books.label ASC;`, keyw)
	if err != nil {
		return errors.Wrap(err, "querying books")
	}
	defer rows.Close()

	infos := []bookInfo{}
	for rows.Next() {
		var info bookInfo
		err = rows.Scan(&info.BookLabel, &info.Archive, &info.NoteCount)
		if err != nil {
			return errors.Wrap(err, "scanning a row")
		}

		infos = append(infos, info)
	}

	for _, info := range infos {
		printBookLine(info, nameOnly)
	}

	return nil
}


func PrintNotes(ctx context.DnoteCtx, bookName string) error {
	db := ctx.DB

	var bookUUID string
	err := db.QueryRow("SELECT uuid FROM books WHERE label = ?", bookName).Scan(&bookUUID)
	if err == sql.ErrNoRows {
		return errors.New("book not found")
	} else if err != nil {
		return errors.Wrap(err, "querying the book")
	}

	rows, err := db.Query(`SELECT rowid, body FROM notes WHERE book_uuid = ? AND deleted = ? ORDER BY added_on ASC;`, bookUUID, false)
	if err != nil {
		return errors.Wrap(err, "querying notes")
	}
	defer rows.Close()

	infos := []noteInfo{}
	for rows.Next() {
		var info noteInfo
		err = rows.Scan(&info.RowID, &info.Body)
		if err != nil {
			return errors.Wrap(err, "scanning a row")
		}

		infos = append(infos, info)
	}

	log.Infof("on book %s\n", bookName)

	for _, info := range infos {
		body, isExcerpt := formatBody(info.Body)

		rowid := log.ColorYellow.Sprintf("(%d)", info.RowID)
		if isExcerpt {
			body = fmt.Sprintf("%s %s", body, log.ColorYellow.Sprintf("[---More---]"))
		}

		log.Plainf("%s %s\n", rowid, body)
	}

	return nil
}

func RetSingle(ctx context.DnoteCtx, bookName string) (string,error) {
	db := ctx.DB

	var bookUUID string
	err := db.QueryRow("SELECT uuid FROM books WHERE label = ?", bookName).Scan(&bookUUID)
	if err == sql.ErrNoRows {
		return "", errors.New("book not found")
	} else if err != nil {
		return "", errors.Wrap(err, "querying the book")
	}

	rows, err := db.Query(`SELECT rowid FROM notes WHERE book_uuid = ? AND deleted = ? ORDER BY added_on ASC;`, bookUUID, false)
	if err != nil {
		return "", errors.Wrap(err, "querying notes")
	}
	defer rows.Close()

	var count int = 0
	var info noteID
	for rows.Next() {
		err = rows.Scan(&info.RowID)
		if err != nil {
			return "", errors.Wrap(err, "scanning a row")
		}
		count += 1
		if count > 1 {
			return "", nil
		}
	}
	return strconv.Itoa(info.RowID), nil
}

func CountBooks(ctx context.DnoteCtx, bookName string) (int,error) {
	db := ctx.DB

	rows, err := db.Query(`SELECT count(notes.uuid) note_count
	FROM books
	LEFT JOIN notes ON notes.book_uuid = books.uuid AND notes.deleted = false
	WHERE books.deleted = false
		AND books.label = ?
	GROUP BY books.uuid;`, bookName)
	if err != nil {
		return 0, errors.Wrap(err, "querying books")
	}
	defer rows.Close()

	var count int 
	var isMany int = 0
	for rows.Next() {
		err = rows.Scan(&count)
		if err != nil {
			return 0, errors.Wrap(err, "scanning a row")
		}
		isMany += 1
		if isMany > 1 {
			return 0, nil
		}
	}

	return count, nil
}
