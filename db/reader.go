package db

import (
	"database/sql"
	"encoding/json"

	"github.com/bakape/meguca/common"
	"github.com/jackc/pgx"
)

var (
	// Don't reallocate this
	emptyArray = []byte("[]")
)

// Open post meta information
type OpenPostMeta struct {
	HasImage  bool   `json:"has_image,omitempty"`
	Spoilered bool   `json:"spoilered,omitempty"`
	Page      uint32 `json:"page"`
	Body      string `json:"body"`
}

// Populate OpenPostMeta from post data
func OpenPostMetaFromPost(p common.Post) (m OpenPostMeta) {
	m = OpenPostMeta{
		Page: p.Page,
		Body: p.Body,
	}
	if p.Image != nil {
		m.HasImage = true
		m.Spoilered = p.Image.Spoiler
	}
	return
}

// GetThread retrieves public thread data from the database.
// page: page of the thread to fetch. -1 to fetch the last page.
func GetThread(id uint64, page int) (thread []byte, err error) {
	err = db.QueryRow("select get_thread($1, $2)", id, page).Scan(&thread)
	castNoRows(&thread, &err)
	return
}

// The PL/pgSQL functions return null on non-existence. Cast that to
// pgx.ErrNoRows.
func castNoRows(buf *[]byte, err *error) {
	if *err == nil && len(*buf) == 0 {
		*err = pgx.ErrNoRows
	}
}

// GetPost reads a single post from the database
func GetPost(id uint64) (post []byte, err error) {
	err = db.
		QueryRow(
			`select encode_post(p)
				|| jsonb_build_object(
					'op', p.op,
					'board', post_board(p.id)
				)
			from posts p
			where p.id = $1`,
			id,
		).
		Scan(&post)
	castNoRows(&post, &err)
	return
}

// GetBoardCatalog retrieves all OPs of a single board
func GetBoardCatalog(board string) (buf []byte, err error) {
	err = db.QueryRow("get_board_catalog").Scan(&buf)
	ensureArray(&buf)
	return
}

// Ensure buf is always an array
func ensureArray(buf *[]byte) {
	if len(*buf) == 0 {
		*buf = emptyArray
	}
}

// GetAllBoardCatalog retrieves all threads for the "/all/" meta-board
func GetAllBoardCatalog() (buf []byte, err error) {
	err = sq.
		Select(
			`jsonb_agg(
				get_thread(id, -6) - 'page'
				order by bump_time desc
			)`,
		).
		From("threads").
		QueryRow().
		Scan(&buf)
	ensureArray(&buf)
	return
}

// Retrieves all threads for a specific board on a specific page
func GetBoard(board string, page uint32) (data []byte, err error) {
	err = db.QueryRow(`select get_board($1, $2)`, board, page).Scan(&data)
	castNoRows(&data, &err)
	return
}

// Retrieves all threads for the "/all/" meta-board on a specific page
func GetAllBoard(page uint32) (board []byte, err error) {
	err = db.QueryRow(`select get_all_board($1)`, page).Scan(&board)
	castNoRows(&board, &err)
	return
}

// Get thread meta-information for initializing thread update feeds
func GetThreadMeta(thread uint64) (
	all map[uint64]uint32,
	open map[uint64]OpenPostMeta,
	moderation map[uint64][]common.ModerationEntry,
	err error,
) {
	// Ensure any pending post body changes for this thread (and also others,
	// while we are at it) are flushed to DB before read
	err = FlushOpenPostBodies()
	if err != nil {
		return
	}

	// TODO: Move this to SQL or PL/pgSQL
	// var buf [3][]byte
	// err = db.
	// 	QueryRow(
	// 		`select
	// 		(),
	// 		(),
	// 		()`,
	// 	).
	// 	Scan(&buf[0], &buf[1], &buf[2])
	// return

	all = make(map[uint64]uint32, 1<<10)
	open = make(map[uint64]OpenPostMeta)
	moderation = make(map[uint64][]common.ModerationEntry)

	err = InTransaction(func(tx *pgx.Tx) (err error) {

		var r *sql.Rows
		defer func() {
			if r != nil {
				r.Close()
			}
		}()

		r, err = sq.
			Select("id", "page").
			From("posts").
			Where("op = ?", thread).
			RunWith(tx).
			Query()
		if err != nil {
			return
		}

		var (
			id   uint64
			page uint32
		)
		for r.Next() {
			err = r.Scan(&id, &page)
			if err != nil {
				return
			}
			all[id] = page
		}
		err = r.Err()
		if err != nil {
			return
		}
		err = r.Close()
		if err != nil {
			return
		}

		r, err = sq.
			Select("id", "sha1 is not null", "spoiler", "page").
			From("posts").
			Where("op = ? and editing = true", thread).
			RunWith(tx).
			Query()
		if err != nil {
			return
		}

		var p OpenPostMeta
		for r.Next() {
			err = r.Scan(&id, &p.HasImage, &p.Spoilered, &p.Page)
			if err != nil {
				return
			}
			open[id] = p
		}
		err = r.Err()
		if err != nil {
			return
		}
		err = r.Close()
		if err != nil {
			return
		}

		r, err = sq.Select("id", "get_post_moderation(id)").
			From("posts").
			Where("op = ? and moderated = true", thread).
			RunWith(tx).
			Query()
		if err != nil {
			return
		}

		var (
			buf []byte
			mod []common.ModerationEntry
		)
		for r.Next() {
			err = r.Scan(&id, &buf)
			if err != nil {
				return
			}
			err = json.Unmarshal(buf, &mod)
			if err != nil {
				return
			}
			copy(moderation[id], mod)
		}
		err = r.Err()
		return
	})
	return
}

// TODO: Board meta for board update feeds.

// Get data assigned on post closure like links and hash command results
func GetPostCloseData(id uint64) (data CloseData, err error) {
	var buf []byte
	err = sq.
		Select(
			`jsonb_build_object(
				'links', get_links(id),
				'commands', commands
			)`,
		).
		From("posts").
		Where("id = ?", id).
		Scan(&buf)
	if err != nil {
		return
	}
	err = json.Unmarshal(buf, &data)
	return
}
