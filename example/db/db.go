package db

import "time"

type UserDB struct {
	ID         int64
	Username   string
	Password   string  // Not included in the DTO
	About      *string  // Non-pointer in the DTO
	Birthday   *time.Time
	CreatedAt  time.Time
}
