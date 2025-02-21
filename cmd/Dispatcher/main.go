package main

import (
	"Dispatcher/internal/config"
	"Dispatcher/internal/entrypoint"
	"fmt"
)

func main() {
	cfg := config.MustLoad()

	fmt.Println(cfg)

	ep, err := entrypoint.New(cfg)
	if err != nil {
		fmt.Println(err)
	}

	err = ep.Run()
	if err != nil {
		fmt.Println(err)
	}
}
