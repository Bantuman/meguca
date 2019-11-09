package db

import (
	"bytes"
	"database/sql"
	"testing"

	"github.com/bakape/meguca/common"
	"github.com/bakape/meguca/config"
	"github.com/bakape/meguca/imager/assets"
	"github.com/bakape/meguca/test"
)

var sampleModerationEntry = common.ModerationEntry{
	Type:   common.BanPost,
	Length: 0,
	By:     "admin",
	Data:   "test",
}

func prepareThreads(t *testing.T) {
	t.Helper()
	assertTableClear(t, "boards", "images")

	boards := [...]BoardConfigs{
		{
			BoardConfigs: config.BoardConfigs{
				ID:        "a",
				Eightball: []string{"yes"},
			},
		},
		{
			BoardConfigs: config.BoardConfigs{
				ID:        "c",
				Eightball: []string{"yes"},
			},
		},
	}
	for _, b := range boards {
		err := InTransaction(func(tx *pgx.Tx) error {
			return WriteBoard(tx, b)
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	threads := [...]Thread{
		{
			ID:         1,
			Board:      "a",
			UpdateTime: 1,
			BumpTime:   1,
		},
		{
			ID:         3,
			Board:      "c",
			UpdateTime: 3,
			BumpTime:   5,
		},
	}
	posts := []Post{
		{
			StandalonePost: common.StandalonePost{
				Post: common.Post{
					ID:    1,
					Image: &assets.StdJPEG,
				},
				OP:    1,
				Board: "a",
			},
			Password: []byte("foo"),
			IP:       "::1",
		},
		{
			StandalonePost: common.StandalonePost{
				Post: common.Post{
					ID: 3,
					Links: map[uint64]common.Link{
						1: {
							OP:    1,
							Board: "a",
						},
					},
					Commands: []common.Command{
						{
							Type: common.Flip,
							Flip: true,
						},
					},
				},
				OP:    3,
				Board: "c",
			},
		},
		{
			StandalonePost: common.StandalonePost{
				Post: common.Post{
					ID:   2,
					Body: "foo",
				},
				OP:    1,
				Board: "a",
			},
		},
	}
	for i := uint64(4); i <= 110; i++ {
		posts = append(posts, Post{
			StandalonePost: common.StandalonePost{
				Post: common.Post{
					ID: i,
				},
				OP:    1,
				Board: "a",
			},
		})
	}

	if err := WriteImage(assets.StdJPEG.ImageCommon); err != nil {
		t.Fatal(err)
	}
	err := InTransaction(func(tx *pgx.Tx) (err error) {
		_, err = tx.Exec(`set constraints links_target_fkey deferred`)
		if err != nil {
			return
		}
		for i := range threads {
			if err := WriteThread(tx, threads[i], posts[i]); err != nil {
				t.Fatal(err)
			}
		}
		for i := len(threads); i < len(posts); i++ {
			if err = WritePost(tx, posts[i]); err != nil {
				return
			}
		}
		return
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sq.Update("posts").Set("moderated", true).Where("id = 1").Exec()
	if err != nil {
		t.Fatal(err)
	}
	s := sampleModerationEntry
	_, err = sq.Insert("post_moderation").
		Columns("post_id", "type", "by", "length", "data").
		Values(1, s.Type, s.By, s.Length, s.Data).
		Exec()
	if err != nil {
		t.Fatal(err)
	}
}

func TestReader(t *testing.T) {
	prepareThreads(t)

	// TODO: Test getting a board index
	// TODO: Test getting /all/ board index
	// TODO: Test getting empty board index
	// TODO: Test getting empty /all/ board index

	t.Run("GetAllCatalog", testGetAllCatalog)
	t.Run("GetCatalog", testGetCatalog)
	t.Run("GetPost", testGetPost)
	t.Run("GetThread", testGetThread)
	t.Run("GetPostCloseData", testGetPostCloseData)
}

func testGetPost(t *testing.T) {
	t.Parallel()

	// Does not exist
	_, err := GetPost(9999)
	if err != pgx.ErrNoRows {
		test.UnexpectedError(t, err)
	}

	// Valid read
	std := common.StandalonePost{
		Post: common.Post{
			ID: 3,
			Links: map[uint64]common.Link{
				1: {
					OP:    1,
					Board: "a",
				},
			},
			Commands: []common.Command{
				{
					Type: common.Flip,
					Flip: true,
				},
			},
		},
		OP:    3,
		Board: "c",
	}
	buf, err := GetPost(3)
	if err != nil {
		t.Fatal(err)
	}
	var p common.StandalonePost
	test.DecodeJSON(t, buf, &p)
	test.AssertEquals(t, p, std)
}

func testGetAllCatalog(t *testing.T) {
	t.Parallel()

	std := map[uint64]common.Thread{
		3: {
			PostCount:  1,
			Board:      "c",
			UpdateTime: 3,
			BumpTime:   5,
			ID:         3,
			Posts: []common.Post{
				{
					ID: 3,
					Links: map[uint64]common.Link{
						1: {
							OP:    1,
							Board: "a",
						},
					},
					Commands: []common.Command{
						{
							Type: common.Flip,
							Flip: true,
						},
					},
				},
			},
		},
		1: {
			ID:         1,
			PostCount:  109,
			ImageCount: 1,
			Board:      "a",
			UpdateTime: 1,
			BumpTime:   1,
			Posts: []common.Post{
				{
					ID:         1,
					Image:      &assets.StdJPEG,
					Moderation: []common.ModerationEntry{sampleModerationEntry},
				},
			},
		},
	}

	buf, err := GetAllBoardCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var catalog []common.Thread
	test.DecodeJSON(t, buf, &catalog)
	for i := range catalog {
		thread := &catalog[i]
		std := std[thread.ID]
		t.Run("assert thread equality", func(t *testing.T) {
			t.Parallel()

			assertImage(t, thread, std.Posts[0].Image)
			syncThreadVariables(thread, std)
			test.AssertEquals(t, thread, &std)
		})
	}
}

// Assert image equality and then override to not compare pointer addresses
// with reflection
func assertImage(t *testing.T, thread *common.Thread, std *common.Image) {
	t.Helper()
	if std != nil {
		if len(thread.Posts) == 0 || thread.Posts[0].Image == nil {
			t.Fatalf("no image on thread %d", thread.ID)
		}
		test.AssertEquals(t, *thread.Posts[0].Image, *std)
		thread.Posts[0].Image = std
	}
}

func testGetCatalog(t *testing.T) {
	t.Parallel()

	cases := [...]struct {
		name, id string
		std      []common.Thread
	}{
		{
			name: "full",
			id:   "c",
			std: []common.Thread{
				{
					ID:         3,
					PostCount:  1,
					Board:      "c",
					UpdateTime: 3,
					BumpTime:   5,
					Posts: []common.Post{
						{
							ID: 3,
							Links: map[uint64]common.Link{
								1: {
									OP:    1,
									Board: "a",
								},
							},
							Commands: []common.Command{
								{
									Type: common.Flip,
									Flip: true,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "empty",
			id:   "z",
			std:  []common.Thread{},
		},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			buf, err := GetBoardCatalog(c.id)
			if err != nil {
				t.Fatal(err)
			}
			var board []common.Thread
			test.DecodeJSON(t, buf, &board)
			for i := range board {
				assertImage(t, &board[i], c.std[i].Posts[0].Image)
			}
			for i := range board {
				syncThreadVariables(&board[i], c.std[i])
			}
			test.AssertEquals(t, board, c.std)
		})
	}
}

// Sync variables that are generated from external state and can not be easily
// tested
func syncThreadVariables(dst *common.Thread, src common.Thread) {
	dst.ID = src.ID
	dst.UpdateTime = src.UpdateTime
	dst.BumpTime = src.BumpTime
}

func testGetThread(t *testing.T) {
	t.Parallel()

	thread1 := common.Thread{
		PostCount:  109,
		ImageCount: 1,
		UpdateTime: 1,
		BumpTime:   1,
		Board:      "a",
		ID:         1,
		Posts: []common.Post{
			{
				ID:         1,
				Image:      &assets.StdJPEG,
				Moderation: []common.ModerationEntry{sampleModerationEntry},
			},
			{
				ID:   2,
				Body: "foo",
			},
		},
	}
	for i := uint64(4); i <= 110; i++ {
		thread1.Posts = append(thread1.Posts, common.Post{
			ID:   i,
			Page: (uint32(i) - 1) / 100,
		})
	}

	firstPage := thread1
	firstPage.Posts = firstPage.Posts[:99]

	last5 := thread1
	last5.Posts = append(
		[]common.Post{thread1.Posts[0]},
		last5.Posts[len(thread1.Posts)-5:]...,
	)

	lastPage := thread1
	lastPage.Page = 1
	lastPage.Posts = append(
		[]common.Post{thread1.Posts[0]},
		lastPage.Posts[99:]...,
	)

	cases := [...]struct {
		name string
		id   uint64
		page int
		std  common.Thread
		err  error
	}{
		{
			name: "first page",
			id:   1,
			std:  firstPage,
		},
		{
			name: "second page",
			id:   1,
			page: 1,
			std:  lastPage,
		},
		{
			name: "last page",
			id:   1,
			page: -1,
			std:  lastPage,
		},
		{
			name: "last 5 replies",
			id:   1,
			page: -5,
			std:  last5,
		},
		{
			name: "no replies ;_;",
			id:   3,
			std: common.Thread{
				Board:      "c",
				UpdateTime: 3,
				BumpTime:   5,
				PostCount:  1,
				ID:         3,
				Posts: []common.Post{
					{
						ID: 3,
						Links: map[uint64]common.Link{
							1: {
								OP:    1,
								Board: "a",
							},
						},
						Commands: []common.Command{
							{
								Type: common.Flip,
								Flip: true,
							},
						},
					},
				},
			},
		},
		{
			name: "nonexistent thread",
			id:   99,
			err:  pgx.ErrNoRows,
		},
	}

	for i := range cases {
		c := cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			buf, err := GetThread(c.id, c.page)
			if err != c.err {
				test.UnexpectedError(t, err)
			}
			if c.err == nil {
				var thread common.Thread
				test.DecodeJSON(t, buf, &thread)
				assertImage(t, &thread, c.std.Posts[0].Image)
				syncThreadVariables(&thread, c.std)
				test.AssertEquals(t, thread, c.std)
			}
		})
	}
}

func testGetPostCloseData(t *testing.T) {
	t.Parallel()

	res, err := GetPostCloseData(3)
	if err != nil {
		t.Fatal(err)
	}

	test.AssertJSON(t, bytes.NewReader(res.Commands), []common.Command{
		{
			Type: common.Flip,
			Flip: true,
		},
	})
	test.AssertEquals(
		t,
		res,
		CloseData{
			Links: map[uint64]common.Link{
				1: {
					OP:    1,
					Board: "a",
				},
			},
			Commands: res.Commands,
		},
	)
}

func TestOpenPostMetaFromPost(t *testing.T) {
	t.Parallel()

	test.AssertEquals(
		t,
		OpenPostMetaFromPost(
			common.Post{
				Page: 1,
				Body: "foo",
				Image: &common.Image{
					Spoiler: true,
				},
			},
		),
		OpenPostMeta{
			Page:      1,
			Body:      "foo",
			HasImage:  true,
			Spoilered: true,
		},
	)
}
