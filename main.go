package main

import (
	"fmt"
	"net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}

func main() {
	imagePath := "car.png"
	session, err := initSession()
	if err != nil {
		fmt.Printf("Failed to initialize session: %v\n", err)
		return
	}
	defer session.Destroy()

	if err := RunModel(session, imagePath); err != nil {
		fmt.Printf("Failed to run model: %v\n", err)
	}

	http.HandleFunc("/", helloHandler)

	fmt.Println("Starting server on :8090")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}
