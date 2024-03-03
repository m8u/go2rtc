package api

import (
	"io"
	"net/http"
	"os"

	"github.com/AlexxIT/go2rtc/internal/app"
	"gopkg.in/yaml.v3"
)

func configHandler(w http.ResponseWriter, r *http.Request) {
	if app.ConfigPath == "" {
		http.Error(w, "", http.StatusGone)
		return
	}

	switch r.Method {
	case "GET":
		data, err := os.ReadFile(app.ConfigPath)
		if err != nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		// https://www.ietf.org/archive/id/draft-ietf-httpapi-yaml-mediatypes-00.html
		Response(w, data, "application/yaml")

	case "POST", "PATCH":
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if r.Method == "PATCH" {
			// no need to validate after merge
			data, err = app.MergeYAML(app.ConfigPath, data)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			// validate config
			if err = yaml.Unmarshal(data, map[string]any{}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if err = os.WriteFile(app.ConfigPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
