package domain

import "context"

type RemoteStorage interface {
	Directory(owner string, repo string, path string, pageNum int, pageSize int) (Page[DirElem], error)
	File(owner string, repo string, path string) (DirElem, error)
	SaveNote(ctx context.Context, owner string, repo string, note Note) error
	UserInfo() (RemoteUser, error)
	UserRepos(username string, pageNum int, pageSize int) (Page[RemoteRepo], error)
	RepoExists(owner string, repo string) (bool, error)
}

type DirElem struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Sha     string `json:"sha"`
}

const TypeDir = "dir"
const TypeFile = "file"

type RemoteUser struct {
	Username string
}

type RemoteRepo struct {
	Name string
}
