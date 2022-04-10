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

package archive

import (
	"github.com/dnote/dnote/pkg/cli/context"
	"github.com/dnote/dnote/pkg/cli/infra"
	"github.com/dnote/dnote/pkg/cli/log"
	"github.com/dnote/dnote/pkg/cli/validate"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	
	//"strconv"
)

var reverseFlag bool

var example = `
 * Archive a book
 dnote archive git

 * Reverse archiving a book
 dnote archive git -reverse`

func preRun(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("Incorrect number of argument")
	}

	return nil
}

// NewCmd returns a new add command
func NewCmd(ctx context.DnoteCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "archive <book>",
		Short:   "Archive a book",
		Aliases: []string{"a"},
		Example: example,
		PreRunE: preRun,
		RunE:    newRun(ctx),
	}

	f := cmd.Flags()
	f.BoolVarP(&reverseFlag, "reverse", "r", false, "Reverse archiving a book")

	return cmd
}

func newRun(ctx context.DnoteCtx) infra.RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		bookName := args[0]
		if err := validate.BookName(bookName); err != nil {
			return errors.Wrap(err, "invalid book name")
		}
		
		tx, err := ctx.DB.Begin()
		if err != nil {
			return errors.Wrap(err, "beginning a transaction")
		}
		
		var bookUUID string
		err = tx.QueryRow("SELECT uuid FROM books WHERE label = ?", bookName).Scan(&bookUUID)
		
		if err != nil {
		return errors.Wrap(err, "finding the book")
		}
		
		if _, err = tx.Exec("UPDATE books SET archive = ? WHERE uuid = ?", !reverseFlag, bookUUID); err != nil {
			tx.Rollback()
			return errors.Wrap(err, "archiving the book")
		}

		err = tx.Commit()
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "comitting transaction")
		}

		if reverseFlag{
			log.Successf("de-archived %s\n", bookName)
		} else {
			log.Successf("archived %s\n", bookName)
		}

		return nil
	}
}

