package db

import (
	"sync"
	"time"

	"github.com/bakape/captchouli"
	"github.com/bakape/meguca/auth"
	"github.com/bakape/meguca/common"
	"github.com/bakape/meguca/config"
	"github.com/jackc/pgx"
)

const (
	// Amount of score, after exceeding which, a captcha solution is requested
	spamDetectionThreshold = time.Minute
)

var (
	spamScoreBuffer = make(map[auth.Token]time.Duration)
	spamMu          sync.RWMutex
)

// Sync cache and DB spam scores
func syncSpamScores() (err error) {
	spamMu.Lock()
	defer spamMu.Unlock()

	if len(spamScoreBuffer) == 0 {
		return
	}
	err = flushSpamScores()
	for user := range spamScoreBuffer {
		delete(spamScoreBuffer, user)
	}
	return
}

// Flush spam scores from buffer to DB
func flushSpamScores() (err error) {
	return InTransaction(func(tx *pgx.Tx) (err error) {
		for user, buffered := range spamScoreBuffer {
			_, err = tx.Exec(
				`insert into spam_scores as s (auth_key, expires)
				values ($1, now() + $2)
				on conflict (auth_key)
				do update set expires = (
					(
						case
							when s.expires < now() then now()
							else s.expires
						end
					)
					+ $2
				)`,
				user,
				buffered,
			)
			if err != nil {
				return
			}
		}
		return
	})
}

// IncrementSpamScore increments spam detection score of a captcha session
// and sends captcha requests, if score exceeded.
//
// user: token identifying user
// increment: increment amount in milliseconds
func IncrementSpamScore(user auth.Token, increment uint) {
	if !config.Get().Captcha {
		return
	}

	spamMu.Lock()
	spamScoreBuffer[user] += time.Duration(increment) * time.Millisecond
	spamMu.Unlock()
}

// NeedCaptcha returns, if the user needs a captcha to proceed with usage
// of server resources
func NeedCaptcha(user auth.Token) (need bool, err error) {
	if !config.Get().Captcha {
		return
	}

	// Require a captcha, if none have been solved in 3 hours
	has, err := SolvedCaptchaRecently(user)
	if err != nil {
		return
	}
	if !has {
		need = true
		return
	}

	score, err := getSpamScore(user)
	if err != nil {
		return
	}
	return score.After(time.Now().Add(spamDetectionThreshold)), err
}

// Merge cached and DB value and return current score
func getSpamScore(user auth.Token) (
	score time.Time,
	err error,
) {
	spamMu.RLock()
	defer spamMu.RUnlock()

	now := time.Now()
	err = db.
		QueryRow(
			`select expires
			from spam_scores
			where auth_key = $1 and expires > now()`,
			user,
		).
		Scan(&score)
	switch err {
	case nil:
	case pgx.ErrNoRows:
		score = now
		err = nil
	default:
		return
	}

	score.Add(spamScoreBuffer[user])

	return
}

// Check if user is spammer
func AssertNotSpammer(user auth.Token) (err error) {
	_, err = getSpamScore(user)
	return
}

// Separated for unit tests
func recordValidCaptcha(user auth.Token) (err error) {
	spamMu.Lock()
	defer spamMu.Unlock()

	delete(spamScoreBuffer, user)

	return InTransaction(func(tx *pgx.Tx) (err error) {
		_, err = tx.Exec(
			`insert into last_solved_captchas (auth_key, expires)
			values ($1, now() + interval '3 hours')
			on conflict (auth_key)
			do update set expires = excluded.expires`,
			user,
		)
		if err != nil {
			return
		}
		_, err = tx.Exec(
			`delete from spam_scores
			where auth_key = $1`,
			user,
		)
		return
	})
}

// ValidateCaptcha with captcha backend
func ValidateCaptcha(req auth.Captcha, user auth.Token) (err error) {
	if !config.Get().Captcha {
		return
	}

	err = captchouli.CheckCaptcha(req.CaptchaID, req.Solution)
	switch err {
	case nil:
		return recordValidCaptcha(user)
	case captchouli.ErrInvalidSolution:
		return common.ErrInvalidCaptcha
	default:
		return
	}
}

// Returns, if user has solved a captcha within the last 3 hours
func SolvedCaptchaRecently(user auth.Token) (
	has bool,
	err error,
) {
	if !config.Get().Captcha {
		has = true
		return
	}

	err = db.
		QueryRow(
			`select exists (
				select
				from last_solved_captchas
				where auth_key = $1 and expires > now()
			)`,
			user,
		).
		Scan(&has)
	return
}
