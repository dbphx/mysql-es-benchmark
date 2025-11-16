package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	// Lấy env
	dsn := os.Getenv("MYSQL_DSN")
	esAddr := os.Getenv("OPENSEARCH_ADDR")
	if dsn == "" {
		dsn = "root:root@tcp(mysql:3306)/testdb"
	}
	if esAddr == "" {
		esAddr = "http://opensearch:9200"
	}

	// Connect MySQL
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Connect OpenSearch
	es, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{esAddr},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Chờ OpenSearch ready (nếu cần)
	for i := 0; i < 5; i++ {
		res, err := es.Cluster.Health()
		if err == nil && res.StatusCode == 200 {
			break
		}
		log.Println("Waiting for OpenSearch...")
		time.Sleep(2 * time.Second)
	}

	ctx := context.Background()

	// Tạo index users nếu chưa tồn tại
	exists, err := es.Indices.Exists([]string{"users"})
	if err != nil {
		log.Fatal(err)
	}
	if exists.StatusCode == 404 {
		log.Println("Creating index 'users'")
		createIndex := `{
			"mappings": {
				"properties": {
					"id": { "type": "integer" },
					"name": { "type": "text" }
				}
			}
		}`
		res, err := es.Indices.Create("users", es.Indices.Create.WithBody(bytes.NewReader([]byte(createIndex))))
		if err != nil {
			log.Fatal(err)
		}
		res.Body.Close()
	}

	// Lấy dữ liệu từ MySQL
	rows, err := db.Query("SELECT id, name FROM users")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	// Bulk index
	bulkIndexer, err := opensearchutil.NewBulkIndexer(opensearchutil.BulkIndexerConfig{
		Client: es,
		Index:  "users",
	})
	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			log.Println("Scan error:", err)
			continue
		}

		data, err := json.Marshal(u)
		if err != nil {
			log.Println("JSON marshal error:", err)
			continue
		}

		reader := bytes.NewReader(data)

		err = bulkIndexer.Add(ctx, opensearchutil.BulkIndexerItem{
			Action:     "index",
			DocumentID: fmt.Sprint(u.ID),
			Body:       reader,
		})
		if err != nil {
			log.Println("Bulk add error:", err)
		}
	}

	if err := bulkIndexer.Close(ctx); err != nil {
		log.Fatal(err)
	}

	stats := bulkIndexer.Stats()
	fmt.Printf("Indexed %d documents, %d errors\n", stats.NumFlushed, stats.NumFailed)
	fmt.Println("Done syncing to OpenSearch")
}
