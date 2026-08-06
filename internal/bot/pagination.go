package bot

const (
	NextPageCommand = "next"
	PrevPageCommand = "prev"
)

type pagePayload struct {
	PageNum int `json:"page"`
}
