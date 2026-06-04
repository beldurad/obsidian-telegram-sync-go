package app

var (
	StateDefault              = 0
	StateWaitingTemplateText  = 1
	StateWaitingTemplateAlias = 2
	StateWaitingFilePath      = 3
	StateWaitingFilePathAlias = 4
	StateWaitingNoteFilePath  = 5
	StateWaitingNoteTemplate  = 6
)

type UserState int

func (u UserState) Validate() bool {
	return u >= 0 && u <= 6
}

type UserStateService interface {
	State(username string) (UserState, error)
	ChangeState(username string, state UserState) error
}
