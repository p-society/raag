package streaming

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	mux "github.com/gorilla/mux"
)

func StartStreaming() error {
	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Raag streaming server")
	})

	r.HandleFunc("/stream/{file}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		file := vars["file"]
		path := filepath.Join("./music", file)

		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}

		http.ServeFile(w, r, path)
	})

	fmt.Println("Starting streaming server on http://localhost:6969")
	return http.ListenAndServe(":6969", r)
}
