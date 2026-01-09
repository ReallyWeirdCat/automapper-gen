package dtos

type Interest int

const (
	Gaming Interest = iota
	Programming
	Cooking
	Painting
	Movies
)

type Role int

const (
	Client Role = iota
	Cashier
	Restocker
	Delivery
	Security
	Tech
)

//automapper:from=db.UserDB
//automapper:bidirectional
type UserDTO struct {
	ID                  int64
	Username            string
	Role                Role `automapper:"converter=StrRoleToEnum"`
	About               string
	Pets                []PetDTO       `automapper:"dto=PetDTO"`
	FeaturedAchievement AchievementDTO `automapper:"dto=AchievementDTO"`
	Interests           []Interest     `automapper:"converter=StrInterestsToEnums"`
	Birthday            *string        `automapper:"converter=TimeToString"`
	CreatedAt           string         `automapper:"converter=TimeToString"`
}

//automapper:from=db.PetDB
type PetDTO struct {
	ID        int64
	Name      string
	Interests []Interest `automapper:"converter=StrInterestsToEnums"`
	Birthday  *string    `automapper:"converter=TimeToString"`
	CreatedAt string     `automapper:"converter=TimeToString"`
}

//automapper:from=db.AchievementDB
//automapper:bidirectional
type AchievementDTO struct {
	ID          int64
	Title       string
	Description string `automapper:"converter=ToLower"`
}
