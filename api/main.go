package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	_ "github.com/go-sql-driver/mysql"
)

var (
	db *sql.DB
	es *elasticsearch.Client
)

func main() {
	var err error
	db, err = sql.Open("mysql", "root:root@tcp(mysql:3306)/testdb")
	if err != nil {
		log.Fatal(err)
	}

	es, err = elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/search-sql", searchSQL)
	http.HandleFunc("/search-es", searchES)

	log.Println("API running on :8080")
	http.ListenAndServe(":8080", nil)
}

func searchSQL(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("q")
	var id int
	var uname string
	err := db.QueryRow("SELECT id, name FROM users WHERE name=? LIMIT 1", name).Scan(&id, &uname)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	fmt.Fprintf(w, "SQL Result: %d %s", id, uname)
}

func searchES(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("q")
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex("users"),
		es.Search.WithQuery(fmt.Sprintf("name:%s", name)),
		es.Search.WithSize(1),
	)
	if err != nil {
		http.Error(w, "ES Error", 500)
		return
	}
	defer res.Body.Close()
	fmt.Fprintf(w, "ES Result: %s", res)
}
