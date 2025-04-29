package server

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/2OldtoDieYoung/final13-14ver2/pgk/api"
	"github.com/2OldtoDieYoung/final13-14ver2/pgk/db"
)

const webDir = "./web"

func StartServer() {
	envDBFILE := os.Getenv("TODO_DBFILE")
	db, err := db.CreateDB(envDBFILE)
	if err != nil {
		log.Printf("Ошибка подключения к БД")
		fmt.Println(err)
		return
	}
	defer db.Close()

	http.Handle("/", http.FileServer(http.Dir(webDir)))
	http.HandleFunc("/api/nextdate", api.NextDateHandler)
	http.HandleFunc("/api/task", func(w http.ResponseWriter, req *http.Request) {
		api.TaskHandler(w, req, db)
	})
	http.HandleFunc("/api/tasks", func(w http.ResponseWriter, req *http.Request) {
		api.GetTasksHandler(w, req, db)
	})
	http.HandleFunc("/api/task/done", func(w http.ResponseWriter, req *http.Request) {
		api.TaskDoneDeleteHandler(w, req, db)
	})

	envPort := os.Getenv("TODO_PORT")
	if envPort == "" {
		envPort = "7540"
	}

	fmt.Printf("Запускаем сервер на порту: %s", envPort)

	err = http.ListenAndServe(":"+envPort, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("Завершаем работу")
}
