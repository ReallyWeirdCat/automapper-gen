package dtos

import (
	"errors"
	"strings"
	"time"
)

// We can define custom converter functions here

// Convert string to Role enum
//automapper:converter
func StrRoleToEnum(roleStr string) (Role, error) {
	switch strings.ToLower(roleStr) {
	case "client":
		return Client, nil
	case "cashier":
		return Cashier, nil
	case "restocker":
		return Restocker, nil
	case "delivery":
		return Delivery, nil
	case "security":
		return Security, nil
	case "tech":
		return Tech, nil
	}
	return 0, errors.New("invalid role: " + roleStr)
}

// Convert array of strings to array of Interest enums
//automapper:converter
func StrInterestsToEnums(interestsStr []string) ([]Interest, error) {
	var interests []Interest // Slice to store converted interests

	for _, interestStr := range interestsStr {
		switch strings.ToLower(interestStr) {
		case "gaming":
			interests = append(interests, Gaming)
		case "programming":
			interests = append(interests, Programming)
		case "cooking":
			interests = append(interests, Cooking)
		case "painting":
			interests = append(interests, Painting)
		case "movies":
			interests = append(interests, Movies)
		default:
			return nil, errors.New("invalid interest: " + interestStr)
		}
	}
	return interests, nil
}

// TimeToJSString converts time.Time to JavaScript ISO 8601 string
//automapper:converter
func TimeToString(t time.Time) string {
	return t.Format(time.RFC3339)
}

// StringToTime converts a JavaScript ISO 8601 string to time.Time
//automapper:inverter=TimeToString
func StringToTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errors.New("invalid time format")
	}
	return t, nil
}

// Converts string to lowercase
//automapper:converter
func ToLower(s string) string {
	return strings.ToLower(s)
}
