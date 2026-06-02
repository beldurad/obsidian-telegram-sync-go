package app

// Note represents Obsidian note to add to the vault
type Note struct {
	Text string
	Template
	FilePath
}

func (n *Note) Validate() bool {
	if n.Text == "" {
		return false
	}
	if !n.Template.Validate() {
		return false
	}
	return true
}

type NoteService interface {
	Save(*Note) error
}
