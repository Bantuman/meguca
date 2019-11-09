// Various periodic cleanup scripts and such

package db

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/bakape/meguca/auth"
	"github.com/bakape/meguca/common"
	"github.com/bakape/meguca/config"
	"github.com/bakape/meguca/parser"
	"github.com/go-playground/log"
	"github.com/jackc/pgx"
)

// Run database clean up tasks at server start and regular intervals. Must be
// launched in separate goroutine.
func runCleanupTasks() {

	sec := time.Tick(time.Second)
	min := time.Tick(time.Minute)
	hour := time.Tick(time.Hour)

	// To ensure even the once an hour tasks are run shortly after server start
	go func() {
		time.Sleep(time.Minute)
		runHourTasks()
	}()

	for {
		select {
		case <-sec:
			logError("flush open post bodies", FlushOpenPostBodies())
			logError("spam score buffer flush", syncSpamScores())
		case <-min:
			if config.Server.ImagerMode != config.ImagerOnly {
				logError("open post cleanup", closeDanglingPosts())
			}

			_, err := db.Exec("clean_up_expiries")
			logError("expired row cleanup", err)
		case <-hour:
			runHourTasks()
		}
	}
}

func runHourTasks() {
	if config.Server.ImagerMode != config.ImagerOnly {
		logError("remove identity info", removeIdentityInfo())
		logError("thread cleanup", deleteOldThreads())
		logError("board cleanup", deleteUnusedBoards())
		_, err := db.Exec(`vacuum`)
		logError("vaccum database", err)
	}
	if config.Server.ImagerMode != config.NoImager {
		logError("image cleanup", deleteUnusedImages())
	}
}

func logError(prefix string, err error) {
	if err != nil {
		log.Errorf("%s: %s: %#v", prefix, err, err)
	}
}

// Remove poster-identifying info from posts older than 7 days
func removeIdentityInfo() error {
	_, err := sq.Update("posts").
		Set("ip", nil).
		Set("password", nil).
		Where(`time < extract(epoch from now() at time zone 'utc'
			- interval '7 days')`).
		Where("ip is not null").
		Exec()
	return err
}

// Close any open posts that have not been closed for 30 minutes
func closeDanglingPosts() error {
	type post struct {
		id          uint64
		board, body string
	}
	var (
		posts []post
		p     post
	)
	err := queryAll(
		sq.Select("id", "board", "body").
			From("posts").
			Where(
				`editing = true
				and time < floor(extract(epoch from now() at time zone 'utc'))
							- 900`,
			).
			OrderBy("id"), // Sort for less page misses on processing
		func(r *sql.Rows) (err error) {
			err = r.Scan(&p.id, &p.board, &p.body)
			if err != nil {
				return err
			}
			posts = append(posts, p)
			return
		},
	)
	if err != nil {
		return err
	}

	for _, p := range posts {
		links, com, err := parser.ParseBody(
			[]byte(p.body),
			config.GetBoardConfigs(p.board).BoardConfigs,
			true,
		)
		switch err.(type) {
		case nil:
		case common.StatusError:
			// Still close posts on invalid input
			if err.(common.StatusError).Code != 400 {
				return err
			}
			err = nil
		default:
			return err
		}
		err = ClosePost(p.id, p.board, p.body, links, com)
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete boards that are older than N days and have not had any new posts for
// N days.
func deleteUnusedBoards() error {
	conf := config.Get()
	if !conf.PruneBoards {
		return nil
	}
	min := time.Now().Add(-time.Duration(conf.BoardExpiry) * time.Hour * 24)
	return InTransaction(func(tx *pgx.Tx) (err error) {
		// Get all inactive boards
		var (
			boards []string
			board  string
		)
		err = queryAll(
			sq.Select("id").
				From("boards").
				Where(`created < ?
					and id != 'all'
					and (
							select coalesce(max(bump_time), 0)
							from threads
							where board = boards.id
						) < ?`,
					min, min.Unix(),
				),
			func(r *sql.Rows) (err error) {
				err = r.Scan(&board)
				if err != nil {
					return
				}
				boards = append(boards, board)
				return
			},
		)
		if err != nil {
			return
		}

		// Delete them and log to global moderation log
		for _, b := range boards {
			err = deleteBoard(tx, b, "system",
				fmt.Sprintf("board %s deleted for inactivity", b))
			if err != nil {
				return
			}
		}
		return
	})
}

func deleteBoard(tx *pgx.Tx, id, by, reason string) (err error) {
	_, err = tx.Exec("delete_board", id)
	if err != nil {
		return
	}
	err = logModeration(tx, auth.ModLogEntry{
		ModerationEntry: common.ModerationEntry{
			Type: common.DeleteBoard,
			By:   by,
			Data: reason,
		},
		Board: "all",
	})
	return
}

// Delete stale threads. Thread retention measured in a bump time threshold,
// that is calculated as a function of post count till bump limit with an N days
// floor and ceiling.
func deleteOldThreads() (err error) {
	conf := config.Get()
	if !conf.PruneThreads {
		return
	}

	return InTransaction(func(tx *pgx.Tx) (err error) {
		// Find threads to delete
		var (
			now           = time.Now().Unix()
			min           = float64(conf.ThreadExpiryMin * 24 * 3600)
			max           = float64(conf.ThreadExpiryMax * 24 * 3600)
			toDel         = make([]uint64, 0, 16)
			id, postCount uint64
			bumpTime      int64
			deleted       sql.NullBool
		)
		err = queryAll(
			sq.
				Select(
					"t.id",
					"bump_time",
					`post_count(t.id)`,
					fmt.Sprintf(
						`(select exists (
							select 1 from post_moderation
							where post_id = t.id and type = %d
						))`,
						common.DeletePost),
				).
				From("threads t").
				Join("posts p on t.id = p.id").
				RunWith(tx),
			func(r *sql.Rows) (err error) {
				err = r.Scan(&id, &bumpTime, &postCount, &deleted)
				if err != nil {
					return
				}
				threshold := min +
					(-max+min)*
						math.Pow(float64(postCount)/common.BumpLimit-1, 3)
				if deleted.Bool {
					threshold /= 3
				}
				if threshold < min {
					threshold = min
				}
				if float64(now-bumpTime) > threshold {
					toDel = append(toDel, id)
				}
				return
			},
		)
		if err != nil {
			return
		}

		var q *sql.Stmt
		if len(toDel) != 0 {
			// Deleted any matched threads
			q, err = tx.Prepare(`delete from threads where id = $1`)
			if err != nil {
				return
			}
			for _, id := range toDel {
				_, err = q.Exec(id)
				if err != nil {
					return
				}
			}
		}

		return
	})
}
