package core

import (
	"math/rand"
	"strings"
	"unicode"
)

// HumanAvatar is the avatar used for human users.
const HumanAvatar = "⏺"

// letterAvatars maps lowercase letters to available avatar options.
// First option is squared letter (🅰), second is circled letter (Ⓐ).
var letterAvatars = map[rune][]string{
	'a': {"🅰", "Ⓐ"},
	'b': {"🅱", "Ⓑ"},
	'c': {"🅲", "Ⓒ"},
	'd': {"🅳", "Ⓓ"},
	'e': {"🅴", "Ⓔ"},
	'f': {"🅵", "Ⓕ"},
	'g': {"🅶", "Ⓖ"},
	'h': {"🅷", "Ⓗ"},
	'i': {"🅸", "Ⓘ"},
	'j': {"🅹", "Ⓙ"},
	'k': {"🅺", "Ⓚ"},
	'l': {"🅻", "Ⓛ"},
	'm': {"🅼", "Ⓜ"},
	'n': {"🅽", "Ⓝ"},
	'o': {"🅾", "Ⓞ"},
	'p': {"🅿", "Ⓟ"},
	'q': {"🆀", "Ⓠ"},
	'r': {"🆁", "Ⓡ"},
	's': {"🆂", "Ⓢ"},
	't': {"🆃", "Ⓣ"},
	'u': {"🆄", "Ⓤ"},
	'v': {"🆅", "Ⓥ"},
	'w': {"🆆", "Ⓦ"},
	'x': {"🆇", "Ⓧ"},
	'y': {"🆈", "Ⓨ"},
	'z': {"🆉", "Ⓩ"},
}

// genericAvatars are used when letter-based avatars are exhausted or unavailable.
var genericAvatars = []string{"✿", "☗", "❖", "⌘", "〶", "☡", "〠", "⏏", "◈", "◉"}

// AssignAvatar returns an avatar for an agent based on their name.
// It tries to match the first letter, falling back to generic avatars.
// usedAvatars contains avatars already assigned to other agents.
func AssignAvatar(agentName string, usedAvatars map[string]struct{}) string {
	if usedAvatars == nil {
		usedAvatars = make(map[string]struct{})
	}

	// Get first letter of agent name
	name := strings.ToLower(agentName)
	if len(name) == 0 {
		return pickUnused(genericAvatars, usedAvatars)
	}

	firstRune := rune(name[0])
	if !unicode.IsLetter(firstRune) {
		return pickUnused(genericAvatars, usedAvatars)
	}

	// Try letter-based avatars first
	if options, ok := letterAvatars[firstRune]; ok {
		for _, avatar := range options {
			if _, used := usedAvatars[avatar]; !used {
				return avatar
			}
		}
	}

	// Fall back to generic avatars
	return pickUnused(genericAvatars, usedAvatars)
}

// pickUnused returns a random unused avatar from the list, or a random one if all are used.
func pickUnused(avatars []string, used map[string]struct{}) string {
	var available []string
	for _, a := range avatars {
		if _, ok := used[a]; !ok {
			available = append(available, a)
		}
	}

	if len(available) > 0 {
		return available[rand.Intn(len(available))]
	}

	// All avatars used, pick any
	return avatars[rand.Intn(len(avatars))]
}

// AllAvatars returns all available avatars for display/selection.
func AllAvatars() []string {
	var all []string
	for _, options := range letterAvatars {
		all = append(all, options...)
	}
	all = append(all, genericAvatars...)
	return all
}

// IsValidAvatar checks if a string is a valid avatar (any single grapheme).
func IsValidAvatar(avatar string) bool {
	if avatar == "" {
		return false
	}
	// Count grapheme clusters (handles multi-byte emoji correctly)
	count := 0
	for range avatar {
		count++
		if count > 2 { // Allow up to 2 runes for emoji with modifiers
			return false
		}
	}
	return true
}
