// Package validation enforces business rules for user-supplied custom aliases
// before they are persisted. It exists as a separate package (rather than
// inline in the handler) so that the same rules can be applied in both the
// API gateway and the gRPC URL service, keeping validation consistent
// regardless of the entry point.
package validation

import (
	"errors"
	"regexp"
	"strings"
)

// Sentinel errors for each validation failure mode. Callers can use
// errors.Is() to present user-facing messages or map to HTTP status codes.
var (
	ErrAliasTooShort     = errors.New("alias must be at least 3 characters")
	ErrAliasTooLong      = errors.New("alias must be at most 50 characters")
	ErrAliasInvalidChars = errors.New("alias can only contain letters, numbers, hyphens, and underscores")
	ErrAliasReserved     = errors.New("alias is reserved and cannot be used")
	ErrAliasProfanity    = errors.New("alias contains inappropriate content")
)

// reservedWords blocks aliases that collide with internal routes and
// well-known URL paths. Without this check, a user could claim "api" or
// "health" as a short link and shadow critical endpoints.
var reservedWords = map[string]bool{
	"api":       true,
	"admin":     true,
	"health":    true,
	"status":    true,
	"metrics":   true,
	"dashboard": true,
	"login":     true,
	"logout":    true,
	"register":  true,
	"auth":      true,
	"static":    true,
	"assets":    true,
	"css":       true,
	"js":        true,
	"img":       true,
	"favicon":   true,
	"robots":    true,
	"sitemap":   true,
	"www":       true,
	"app":       true,
	"mail":      true,
	"ftp":       true,
}

// profanityWords blocks offensive, abusive, and potentially harmful content
// from appearing in short-link aliases. This includes slurs, explicit terms,
// common spam/scam keywords, and social-engineering phrases. The list is
// checked both as exact matches and as substring matches to catch compound
// aliases like "mypornsite".
var profanityWords = map[string]bool{
	"fuck": true, "fucking": true, "fucker": true, "shit": true, "bullshit": true,
	"bitch": true, "bastard": true, "asshole": true, "dick": true, "dumbass": true,
	"piss": true, "pissed": true, "crap": true, "motherfucker": true, "sucker": true,
	"wtf": true, "stfu": true, "retard": true, "idiot": true, "moron": true,

	"porn": true, "xxx": true, "adult": true, "nsfw": true, "nude": true,
	"nudity": true, "boobs": true, "tits": true, "sex": true, "sexy": true,
	"hardcore": true, "fetish": true, "camgirl": true, "onlyfans": true,
	"strip": true, "stripper": true, "escort": true, "bdsm": true,
	"deepthroat": true, "hentai": true, "anal": true, "horny": true,

	"spam": true, "scam": true, "fraud": true, "fake": true, "phishing": true,
	"malware": true, "virus": true, "trojan": true, "crypto scam": true,
	"giveaway": true, "free money": true, "click here": true, "visit link": true,
	"guaranteed profit": true, "limited offer": true, "get rich": true,
	"loan approval": true, "investment scheme": true, "ponzi": true, "pump": true,
	"dump": true, "get followers": true, "win iphone": true,

	"drug": true, "drugs": true, "cocaine": true, "weed": true, "marijuana": true,
	"heroin": true, "lsd": true, "meth": true, "ecstasy": true, "mdma": true,
	"opium": true, "ketamine": true, "ghb": true,

	"casino": true, "betting": true, "bet": true, "sportsbook": true,
	"jackpot": true, "poker": true, "roulette": true, "slots": true,
	"lottery": true, "gambling": true,

	"kill": true, "murder": true, "execute": true, "suicide": true,
	"selfharm": true, "blood": true, "stab": true, "shoot": true,

	"hate": true, "loser": true, "trash": true, "clown": true,
	"pathetic": true, "disgusting": true,

	"forex signals": true, "binary options": true, "crypto miner": true,
	"airdrop": true, "wallet drain": true, "mirror trading": true,
	"mlm": true, "network marketing": true, "referral scam": true,

	"telegram bot": true, "whatsapp bot": true, "dm me": true,
	"seller account": true, "resell": true, "buy followers": true,
	"cheap followers": true, "boost engagement": true,

	"bank login": true, "reset password": true, "verify identity": true,
	"unlock account": true, "suspicious activity": true,
	"confirm your details": true, "security alert": true,

	"weapon": true, "gun": true, "bomb": true, "explosive": true,
}

// aliasRegex enforces a URL-safe character set. Only ASCII letters, digits,
// hyphens, and underscores are allowed. This avoids encoding issues in URLs
// and prevents visually confusing aliases with special characters.
var aliasRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateAlias runs all validation rules against a proposed custom alias.
// Checks are ordered from cheapest to most expensive: length, regex, reserved
// word exact match, profanity exact match, and finally profanity substring
// scan. The function returns the first error encountered (fail-fast) so the
// user gets a single, actionable message.
func ValidateAlias(alias string) error {
	lowerAlias := strings.ToLower(alias)

	if len(alias) < 3 {
		return ErrAliasTooShort
	}
	if len(alias) > 50 {
		return ErrAliasTooLong
	}

	if !aliasRegex.MatchString(alias) {
		return ErrAliasInvalidChars
	}

	if reservedWords[lowerAlias] {
		return ErrAliasReserved
	}

	if profanityWords[lowerAlias] {
		return ErrAliasProfanity
	}

	for word := range profanityWords {
		if strings.Contains(lowerAlias, word) {
			return ErrAliasProfanity
		}
	}

	return nil
}

// SuggestAlternatives generates simple numeric suffixed variations of an alias
// (e.g., "mylink-1", "mylink-2") to help the user pick an available name when
// their first choice is taken. The suggestions are deterministic -- they
// always append "-1" through "-N" -- which keeps the implementation simple.
// A future improvement could check the database to guarantee availability.
func SuggestAlternatives(alias string, count int) []string {
	suggestions := make([]string, 0, count)

	for i := 1; i <= count; i++ {
		suggestions = append(suggestions, alias+"-"+string(rune('0'+i)))
	}

	return suggestions
}
