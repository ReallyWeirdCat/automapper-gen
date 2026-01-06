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
		ID:        69,
		Username:  "Nice",
		Password:  "123",
		Role:      "security",
		Interests: []string{"cooking", "movies"},
		About:     nil,
		Birthday:  &birthday,
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
    // User: {ID:69 Username:Nice Role:4 About: Interests:[2 4] Birthday:0xc000014070 CreatedAt:2026-01-06T19:54:25+03:00}
}
