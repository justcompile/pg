package pg_test

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/pg.v4"
)

var db *pg.DB

func init() {
	db = pg.Connect(&pg.Options{
		User: "postgres",
	})
}

func connectDB() *pg.DB {
	db := pg.Connect(&pg.Options{
		User: "postgres",
	})

	err := createTestSchema(db)
	if err != nil {
		panic(err)
	}

	err = db.Create(&Book{
		Title:     "book 1",
		AuthorID:  10,
		EditorID:  11,
		CreatedAt: time.Now(),
	})
	if err != nil {
		panic(err)
	}

	err = db.Create(&Book{
		Title:     "book 2",
		AuthorID:  10,
		EditorID:  12,
		CreatedAt: time.Now(),
	})
	if err != nil {
		panic(err)
	}

	err = db.Create(&Book{
		Title:     "book 3",
		AuthorID:  11,
		EditorID:  11,
		CreatedAt: time.Now(),
	})
	if err != nil {
		panic(err)
	}

	return db
}

func ExampleConnect() {
	db := pg.Connect(&pg.Options{
		User: "postgres",
	})
	err := db.Close()
	fmt.Println(err)
	// Output: <nil>
}

func ExampleDB_QueryOne() {
	var user struct {
		Name string
	}

	res, err := db.QueryOne(&user, `
        WITH users (name) AS (VALUES (?))
        SELECT * FROM users
    `, "admin")
	if err != nil {
		panic(err)
	}
	fmt.Println(res.Affected())
	fmt.Println(user)
	// Output: 1
	// {admin}
}

func ExampleDB_QueryOne_returning_id() {
	_, err := db.Exec(`CREATE TEMP TABLE users(id serial, name varchar(500))`)
	if err != nil {
		panic(err)
	}

	var user struct {
		Id   int32
		Name string
	}
	user.Name = "admin"

	_, err = db.QueryOne(&user, `
        INSERT INTO users (name) VALUES (?name) RETURNING id
    `, user)
	if err != nil {
		panic(err)
	}
	fmt.Println(user)
	// Output: {1 admin}
}

func ExampleDB_QueryOne_Scan() {
	var s1, s2 string
	_, err := db.QueryOne(pg.Scan(&s1, &s2), `SELECT ?, ?`, "foo", "bar")
	fmt.Println(s1, s2, err)
	// Output: foo bar <nil>
}

func ExampleDB_Exec() {
	res, err := db.Exec(`CREATE TEMP TABLE test()`)
	fmt.Println(res.Affected(), err)
	// Output: -1 <nil>
}

func ExampleListener() {
	ln, err := db.Listen("mychan")
	if err != nil {
		panic(err)
	}

	wait := make(chan struct{}, 2)
	go func() {
		wait <- struct{}{}
		channel, payload, err := ln.Receive()
		fmt.Printf("%s %q %v", channel, payload, err)
		wait <- struct{}{}
	}()

	<-wait
	db.Exec("NOTIFY mychan, ?", "hello world")
	<-wait

	// Output: mychan "hello world" <nil>
}

func txExample() *pg.DB {
	db := pg.Connect(&pg.Options{
		User: "postgres",
	})

	queries := []string{
		`DROP TABLE IF EXISTS tx_test`,
		`CREATE TABLE tx_test(counter int)`,
		`INSERT INTO tx_test (counter) VALUES (0)`,
	}
	for _, q := range queries {
		_, err := db.Exec(q)
		if err != nil {
			panic(err)
		}
	}

	return db
}

func ExampleDB_Begin() {
	db := txExample()

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	var counter int
	_, err = tx.QueryOne(pg.Scan(&counter), `SELECT counter FROM tx_test`)
	if err != nil {
		tx.Rollback()
		panic(err)
	}

	counter++

	_, err = tx.Exec(`UPDATE tx_test SET counter = ?`, counter)
	if err != nil {
		tx.Rollback()
		panic(err)
	}

	err = tx.Commit()
	if err != nil {
		panic(err)
	}

	fmt.Println(counter)
	// Output: 1
}

func ExampleDB_RunInTransaction() {
	db := txExample()

	var counter int
	// Transaction is automatically rollbacked on error.
	err := db.RunInTransaction(func(tx *pg.Tx) error {
		_, err := tx.QueryOne(pg.Scan(&counter), `SELECT counter FROM tx_test`)
		if err != nil {
			return err
		}

		counter++

		_, err = tx.Exec(`UPDATE tx_test SET counter = ?`, counter)
		return err
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(counter)
	// Output: 1
}

func ExampleDB_Prepare() {
	stmt, err := db.Prepare(`SELECT $1::text, $2::text`)
	if err != nil {
		panic(err)
	}

	var s1, s2 string
	_, err = stmt.QueryOne(pg.Scan(&s1, &s2), "foo", "bar")
	fmt.Println(s1, s2, err)
	// Output: foo bar <nil>
}

func ExampleInts() {
	var nums pg.Ints
	_, err := db.Query(&nums, `SELECT generate_series(0, 10)`)
	fmt.Println(nums, err)
	// Output: [0 1 2 3 4 5 6 7 8 9 10] <nil>
}

func ExampleInts_in() {
	ids := pg.Ints{1, 2, 3}
	q, err := pg.FormatQuery(`SELECT * FROM table WHERE id IN (?)`, ids)
	fmt.Println(string(q), err)
	// Output: SELECT * FROM table WHERE id IN (1,2,3) <nil>
}

func ExampleStrings() {
	var strs pg.Strings
	_, err := db.Query(
		&strs, `WITH users AS (VALUES ('foo'), ('bar')) SELECT * FROM users`)
	fmt.Println(strs, err)
	// Output: [foo bar] <nil>
}

func ExampleDB_CopyFrom() {
	_, err := db.Exec(`CREATE TEMP TABLE words(word text, len int)`)
	if err != nil {
		panic(err)
	}

	r := strings.NewReader("hello,5\nfoo,3\n")
	_, err = db.CopyFrom(r, `COPY words FROM STDIN WITH CSV`)
	if err != nil {
		panic(err)
	}

	buf := &bytes.Buffer{}
	_, err = db.CopyTo(&NopWriteCloser{buf}, `COPY words TO STDOUT WITH CSV`)
	if err != nil {
		panic(err)
	}
	fmt.Println(buf.String())
	// Output: hello,5
	// foo,3
}

func ExampleDB_WithTimeout() {
	var count int
	// Use bigger timeout since this query is known to be slow.
	_, err := db.WithTimeout(time.Minute).QueryOne(pg.Scan(&count), `
		SELECT count(*) FROM big_table
	`)
	if err != nil {
		panic(err)
	}
}

func ExampleDB_Create() {
	db := connectDB()

	book := Book{
		Title:    "new book",
		AuthorID: 1,
	}

	err := db.Create(&book)
	if err != nil {
		panic(err)
	}
	fmt.Println(book)
	// Output: Book<Id=4 Title="new book">

	err = db.Delete(&book)
	if err != nil {
		panic(err)
	}
}

func ExampleDB_Model_selectFirstAndLastRow() {
	db := connectDB()

	var firstBook Book
	err := db.Model(&firstBook).First()
	if err != nil {
		panic(err)
	}

	var lastBook Book
	err = db.Model(&lastBook).Last()
	if err != nil {
		panic(err)
	}

	fmt.Println(firstBook, lastBook)
	// Output: Book<Id=1 Title="book 1"> Book<Id=3 Title="book 3">
}

func ExampleDB_Model_selectAllColumns() {
	db := connectDB()

	var book Book
	err := db.Model(&book).Columns("book.*").First()
	if err != nil {
		panic(err)
	}
	fmt.Println(book, book.AuthorID)
	// Output: Book<Id=1 Title="book 1"> 10
}

func ExampleDB_Model_selectSomeColumns() {
	db := connectDB()

	var book Book
	err := db.Model(&book).
		Columns("book.id").
		First()
	if err != nil {
		panic(err)
	}

	fmt.Println(book)
	// Output: Book<Id=1 Title="">
}

func ExampleDB_Model_count() {
	db := connectDB()

	count, err := db.Model(Book{}).Count()
	if err != nil {
		panic(err)
	}

	fmt.Println(count)
	// Output: 3
}

func ExampleDB_Update() {
	db := connectDB()

	err := db.Update(&Book{
		Id:    1,
		Title: "updated book 1",
	})
	if err != nil {
		panic(err)
	}

	var book Book
	err = db.Model(&book).Where("id = ?", 1).Select()
	if err != nil {
		panic(err)
	}

	fmt.Println(book)
	// Output: Book<Id=1 Title="updated book 1">
}

func ExampleDB_Update_someColumns() {
	db := connectDB()

	book := Book{
		Id:       1,
		Title:    "updated book 1",
		AuthorID: 2,
	}
	err := db.Model(&book).Columns("title").Returning("*").Update()
	if err != nil {
		panic(err)
	}

	fmt.Println(book, book.AuthorID)
	// Output: Book<Id=1 Title="updated book 1"> 10
}

func ExampleDB_Update_usingSqlFunction() {
	db := connectDB()

	id := 1
	data := map[string]interface{}{
		"title": pg.Q("concat(?, title, ?)", "prefix ", " suffix"),
	}
	var book Book
	err := db.Model(&book).
		Where("id = ?", id).
		Returning("*").
		UpdateValues(data)
	if err != nil {
		panic(err)
	}

	fmt.Println(book)
	// Output: Book<Id=1 Title="prefix book 1 suffix">
}

func ExampleDB_Update_multipleRows() {
	db := connectDB()

	ids := pg.Ints{1, 2}
	data := map[string]interface{}{
		"title": pg.Q("concat(?, title, ?)", "prefix ", " suffix"),
	}

	var books []Book
	err := db.Model(&books).
		Where("id IN (?)", ids).
		Returning("*").
		UpdateValues(data)
	if err != nil {
		panic(err)
	}

	fmt.Println(books[0], books[1])
	// Output: Book<Id=1 Title="prefix book 1 suffix"> Book<Id=2 Title="prefix book 2 suffix">
}

func ExampleDB_Delete() {
	db := connectDB()

	book := Book{
		Title:    "title 1",
		AuthorID: 1,
	}
	err := db.Create(&book)
	if err != nil {
		panic(err)
	}

	err = db.Delete(book)
	if err != nil {
		panic(err)
	}

	err = db.Model(&Book{}).Where("id = ?", book.Id).First()
	fmt.Println(err)
	// Output: pg: no rows in result set
}

func ExampleDB_Delete_multipleRows() {
	db := connectDB()

	ids := pg.Ints{1, 2, 3}
	err := db.Model(Book{}).Where("id IN (?)", ids).Delete()
	if err != nil {
		panic(err)
	}

	count, err := db.Model(Book{}).Count()
	if err != nil {
		panic(err)
	}

	fmt.Println(count)
	// Output: 0
}
