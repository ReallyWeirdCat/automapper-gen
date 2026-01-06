package db

import "time"

type UserDB struct {
	ID                 int64
	Username           string
	Role               string         // DTO wants Role
	Interests          []string       // DTO wants []Interest
	Password           string         // Not included in the DTO
	About              *string        // Non-pointer in the DTO
	FeaturedAchievement *AchievementDB // Wanted as AchievementDTO
	Pets               []PetDB        // Wanted as an array of PetDTO
	Birthday           *time.Time
	CreatedAt          time.Time
}

type PetDB struct {
	ID        int64
	Name      string
	Interests []string // DTO wants []Interest
	Birthday  *time.Time
	CreatedAt time.Time
}

type AchievementDB struct {
	ID          int64
	Title       string
	Description string
	CreatedAt   time.Time
}
