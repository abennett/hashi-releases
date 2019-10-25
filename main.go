package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

var releaseDB *Releases

func main() {
	releaseDB = NewReleases()
	log.Println("Fetching all releases")
	_, err := releaseDB.FetchAllReleases()
	if err != nil {
		panic(err)
	}
	log.Print("Fetched all releases")
	releaseDB.BuildReleaseDB()
	log.Println("DB built")
	r := Route()
	log.Println("serving on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func Route() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Get("/", handleRoot)
	r.Get("/latest/{product}", handleProductLatest)
	r.Get("/versions/{product}", handleListVersions)
	r.Get("/list", handleListProducts)
	return r
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		AvailableRoutes []string `json:"available_routes"`
	}{
		AvailableRoutes: []string{
			"list",
			"/latest/{product}",
			"/versions/{product}",
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func handleListProducts(w http.ResponseWriter, r *http.Request) {
	products := releaseDB.ListProducts()
	resp := struct {
		Products []string `json:"products"`
	}{
		Products: products,
	}
	json.NewEncoder(w).Encode(resp)
}

func handleProductLatest(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version, ok := releaseDB.LatestVersion(product)
	if !ok {
		http.Error(w, product+" not found", http.StatusNotFound)
		return
	}
	resp := struct {
		Product string `json:"product"`
		Version string `json:"version"`
	}{
		Product: product,
		Version: version,
	}
	json.NewEncoder(w).Encode(resp)
	return
}

func handleListVersions(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	versions, err := releaseDB.ListAllVersions(product)
	if err != nil {
		http.Error(w, product+" not found", http.StatusNotFound)
		return
	}
	resp := struct {
		Product  string   `json:"product"`
		Versions []string `json:"versions"`
	}{
		Product:  product,
		Versions: versions,
	}
	json.NewEncoder(w).Encode(resp)
}
