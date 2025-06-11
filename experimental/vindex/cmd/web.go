// Copyright 2025 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"k8s.io/klog/v2"
)

// Define a struct to hold any data we might pass to the template
type FormData struct {
	Message string
}

var (
	//go:embed templates/form.html
	templateStr string
	tmpl        = template.Must(template.New("form").Parse(templateStr))
)

func NewServer(lookup func(string) string) Server {
	return Server{
		lookup: lookup,
	}
}

type Server struct {
	lookup func(string) string
}

// serveForm handles GET requests to the root path ("/")
// It renders the HTML form.
func (s Server) serveForm(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Render the form template with no initial data
	err := tmpl.Execute(w, FormData{Message: "Enter your string below:"})
	if err != nil {
		klog.Warningf("Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleSubmit handles POST requests to "/submit"
// It parses the form data and responds.
func (s Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing form: %v", err), http.StatusBadRequest)
		return
	}
	inputString := r.FormValue("inputString")

	klog.V(2).Infof("Received string from form: '%s'", inputString)

	responseMessage := s.lookup(inputString)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, responseMessage)
}

func (s Server) registerHandlers(r *mux.Router) {
	r.HandleFunc("/", s.serveForm).Methods("GET")
	r.HandleFunc("/submit", s.handleSubmit).Methods("POST")
}
