package db

import "time"

type UserDB struct {
	ID         int64
	Username   string
	Password   string
	CreatedAt  time.Time
}
