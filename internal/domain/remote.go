package domain

import (
	"context"
	"time"
)

type RemoteConnectCtx struct {
	ID       int64
	State    string
	Verifier string
}

type RemoteToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type RemoteUser struct {
	Username string
}

type RemoteRepo struct {
	Name string
}

type RemoteStorage interface {
	CreateNote(ctx context.Context, owner string, repo string, n Note) error
	Directory(owner string, repo string, path string, pageNum int, pageSize int) (Page[File], error)
	File(ctx context.Context, owner string, repo string, path string) (File, error)
	RepoExists(owner string, repo string) (bool, error)
	UpdateNote(ctx context.Context, owner string, repo string, n Note) error
	UserInfo() (RemoteUser, error)
	UserRepos(username string, pageNum int, pageSize int) (Page[RemoteRepo], error)
}

// [File] represents file in remote repository. If file is directory, Content = ""
type File struct {
	Path    Path
	Name    string
	Content string
}

type Path struct {
	Type  string
	Value string
}

var PathTypeDir = "dir"
var PathTypeFile = "file"
