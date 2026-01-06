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

	pets := []db.PetDB{
		{
			ID:        23,
			Name:      "Hamsterine",
			Interests: []string{"gaming", "cooking"},
		},
		{
			ID:        14,
			Name:      "Froggy",
			Birthday:  &birthday,
			Interests: []string{"movies", "painting"},
		},
	}

	achievement := db.AchievementDB{
		ID:          342,
		Title:       "Employee of the month",
		Description: "Wow what a chad",
	}

	// Pretend that we got this from our database
	user := db.UserDB{
		ID:                  69,
		Username:            "Nice",
		Password:            "123",
		Role:                "security",
		Interests:           []string{"cooking", "movies"},
		Pets:                pets,
		FeaturedAchievement: &achievement,
		About:               nil,
		Birthday:            &birthday,
		CreatedAt:           time.Now(),
	}

	// Let's convert the model to a DTO via the generated method
	var dto dtos.UserDTO
	err := dto.MapFromUserDB(&user)

	// Some field conversions might fail (especially if custom)
	if err != nil {
		panic(fmt.Errorf("Failed to map: %s", err.Error()))
	}

	fmt.Printf("User: %+v\n", dto)
	// User: {ID:69 Username:Nice Role:4 About: Pets:[{ID:23 Name:Hamsterine Interests:[0 2] Birthda
	// y:<nil> CreatedAt:0001-01-01T00:00:00Z} {ID:14 Name:Froggy Interests:[4 3] Birthday:0xc00011c
	// 020 CreatedAt:0001-01-01T00:00:00Z}] FeaturedAchievement:{ID:342 Title:Employee of the month
	// Description:Wow what a chad CreatedAt:0001-01-01T00:00:00Z} Interests:[2 4] Birthday:0xc00011
	// c030 CreatedAt:2026-01-06T22:59:52+03:00}
}
