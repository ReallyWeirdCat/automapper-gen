package dtos

import (
	"errors"
	"strings"
	"time"
)

// We can define custom converter functions here

// Convert string to Role enum
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
func TimeToJSString(t time.Time) string {
	return t.Format(time.RFC3339)
}

// Converts string to lowercase
func ToLower(s string) string {
	return strings.ToLower(s)
}
