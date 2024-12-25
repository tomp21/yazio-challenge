package reconcilers

import (
	"strings"
	"testing"
)

func Test_generateSecret(t *testing.T) {
	password := generateSecureRedisPassword()

	if len(password) != passwordLength {
		t.Fatalf("Password length is not the expected one")
	}
	var succeed, specialChar, lowerCaseLetter, upperCaseLetter, number bool
	for _, rune := range password {
		char := string(rune)
		if strings.Contains(passwordSpecialChars, char) {
			specialChar = true
		}
		if strings.Contains(passwordLetters, char) {
			lowerCaseLetter = true
		}
		if strings.Contains(passwordNumbers, char) {
			number = true
		}
		if strings.Contains(passwordLetters, strings.ToLower(char)) {
			upperCaseLetter = true
		}
		succeed = specialChar && lowerCaseLetter && number && upperCaseLetter
	}
	if !succeed {
		t.Fatalf("Password %s did not contain all required types of characters", password)
	}
}
