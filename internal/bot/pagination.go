package bot

const (
	NextPageCallback   = "next"
	NextPageButtonText = ">"

	PrevPageCallback   = "prev"
	PrevPageButtonText = "<"
)

type pagePayload struct {
	PageNum int `json:"page"`
}
