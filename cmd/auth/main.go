package main

import (
	"log"
	"microservices_project/internal/auth"
)

func main() {
	if err := auth.Run(); err != nil {
		log.Fatal(err)
	}
}
