package dtos

//automapper:from=db.UserDB
type UserDTO struct {
	ID        int64
	Username  string
	CreatedAt string `automapper:"converter=jsTime"`
}
