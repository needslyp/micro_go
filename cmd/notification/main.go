package main

import (
	"log"
	"microservices_project/internal/notification"
)

func main() {
	if err := notification.Run(); err != nil {
		log.Fatal(err)
	}
}
