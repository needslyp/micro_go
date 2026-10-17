package main

import (
	"log"
	"microservices_project/internal/blog"
)

func main() {
	if err := blog.Run(); err != nil {
		log.Fatal(err)
	}
}
