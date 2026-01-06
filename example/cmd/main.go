package main

import (
	"fmt"
	"time"

	"git.weirdcat.su/weirdcat/automapper-gen/example/db"
	"git.weirdcat.su/weirdcat/automapper-gen/example/dtos"
)

func main() {

	// Classic Steam user birthday
	birthday := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)

	// Pretend that we got this from our database
	user := db.UserDB{
		ID: 69,
		Username: "Nice",
		Password: "123",
		About: nil,
		Birthday: &birthday,
		CreatedAt: time.Now(),
	}

	// Let's convert the model to a DTO via the generated method
	var dto dtos.UserDTO
	err := dto.MapFromUserDB(&user)

	// Some field conversions might fail (especially if custom)
	if err != nil {
		panic(fmt.Errorf("Failed to map: %s", err.Error()))
	}

	fmt.Printf("User: %+v\n", dto)
	// User: {ID:69 Username:Nice About: Birthday:0xc000014070 CreatedAt:2026-01-06T05:05:23+03:00}
}
