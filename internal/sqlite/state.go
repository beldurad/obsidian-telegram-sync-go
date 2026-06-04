package sqlite

import (
	"database/sql"

	app "github.com/beldurad/obsidian-telegram-sync-go/internal"
)

type UserStateService struct {
	db *sql.DB
}

func NewUserStateService(db *sql.DB) *UserStateService {
	return &UserStateService{
		db: db,
	}
}

func (s *UserStateService) State(username string) (app.UserState, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	state, err := stateByUsername(tx, username)
	return state, err
}

func (s *UserStateService) ChangeState(username string, state app.UserState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	err = changeStateByUsername(tx, username, state)
	return err
}

func stateByUsername(tx *sql.Tx, username string) (app.UserState, error) {
	const query = `
	SELECT state
	FROM user_state
	WHERE username = $1
	`
	row := tx.QueryRow(query, username)

	var state int
	if err := row.Scan(&state); err != nil {
		return 0, err
	}
	return app.UserState(state), nil
}

func changeStateByUsername(tx *sql.Tx, username string, state app.UserState) error {
	const query = `
	INSERT INTO user_state (username, state)
	VALUES ($1, $2)
	ON CONFLICT(username)
	DO UPDATE SET
		state = $2
	`
	_, err := tx.Exec(query, username, state)
	return err
}
