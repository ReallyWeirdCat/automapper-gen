package dtos

//automapper:from=db.UserDB
type UserDTO struct {
	ID        int64
	Username  string
	About     string  // pointer mismatch!
	Birthday  *string `automapper:"converter=TimeToString"`
	CreatedAt string `automapper:"converter=TimeToString"`
}
