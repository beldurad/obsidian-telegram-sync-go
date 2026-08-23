package mock

import (
	"context"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

var _ domain.RemoteStorage = (*RemoteStorage)(nil)

type RemoteStorage struct {
	domain.RemoteStorage

	DirectoryFn func(
		owner string,
		repo string,
		path string,
		pageNum int,
		pageSize int,
	) (domain.Page[domain.File], error)

	DirectoryCalled bool
	DirectoryOwner  string
	DirectoryRepo   string
	DirectoryPath   string
	DirectoryPage   int
	DirectorySize   int

	UserInfoFn func() (domain.RemoteUser, error)

	UserInfoCalled bool

	UserReposFn func(
		username string,
		pageNum int,
		pageSize int,
	) (domain.Page[domain.RemoteRepo], error)

	UserReposCalled   bool
	UserReposUsername string
	UserReposPage     int
	UserReposSize     int

	RepoExistsFn func(owner, repo string) (bool, error)

	RepoExistsCalled bool
	RepoExistsOwner  string
	RepoExistsRepo   string

	CreateNoteFn func(
		ctx context.Context,
		owner string,
		repo string,
		note domain.Note,
	) error

	CreateNoteCalled bool
	CreateNoteOwner  string
	CreateNoteRepo   string
	CreatedNote      domain.Note

	FileFn func(
		ctx context.Context,
		owner string,
		repo string,
		path string,
	) (domain.File, error)

	FileCalled bool
	FileOwner  string
	FileRepo   string
	FilePath   string

	UpdateNoteFn func(
		ctx context.Context,
		owner string,
		repo string,
		note domain.Note,
	) error

	UpdateNoteCalled bool
	UpdateNoteOwner  string
	UpdateNoteRepo   string
	UpdatedNote      domain.Note
}

func (m *RemoteStorage) Directory(
	owner string,
	repo string,
	path string,
	pageNum int,
	pageSize int,
) (domain.Page[domain.File], error) {
	m.DirectoryCalled = true
	m.DirectoryOwner = owner
	m.DirectoryRepo = repo
	m.DirectoryPath = path
	m.DirectoryPage = pageNum
	m.DirectorySize = pageSize

	if m.DirectoryFn != nil {
		return m.DirectoryFn(owner, repo, path, pageNum, pageSize)
	}

	return domain.Page[domain.File]{}, nil
}

func (m *RemoteStorage) CreateNote(
	ctx context.Context,
	owner string,
	repo string,
	note domain.Note,
) error {
	m.CreateNoteCalled = true
	m.CreateNoteOwner = owner
	m.CreateNoteRepo = repo
	m.CreatedNote = note

	if m.CreateNoteFn != nil {
		return m.CreateNoteFn(ctx, owner, repo, note)
	}

	return nil
}

func (m *RemoteStorage) File(
	ctx context.Context,
	owner string,
	repo string,
	path string,
) (domain.File, error) {
	m.FileCalled = true
	m.FileOwner = owner
	m.FileRepo = repo
	m.FilePath = path

	if m.FileFn != nil {
		return m.FileFn(ctx, owner, repo, path)
	}

	return domain.File{}, nil
}

func (m *RemoteStorage) UpdateNote(
	ctx context.Context,
	owner string,
	repo string,
	note domain.Note,
) error {
	m.UpdateNoteCalled = true
	m.UpdateNoteOwner = owner
	m.UpdateNoteRepo = repo
	m.UpdatedNote = note

	if m.UpdateNoteFn != nil {
		return m.UpdateNoteFn(ctx, owner, repo, note)
	}

	return nil
}

func (m *RemoteStorage) UserInfo() (domain.RemoteUser, error) {
	m.UserInfoCalled = true

	if m.UserInfoFn != nil {
		return m.UserInfoFn()
	}

	return domain.RemoteUser{}, nil
}

func (m *RemoteStorage) UserRepos(
	username string,
	pageNum int,
	pageSize int,
) (domain.Page[domain.RemoteRepo], error) {
	m.UserReposCalled = true
	m.UserReposUsername = username
	m.UserReposPage = pageNum
	m.UserReposSize = pageSize

	if m.UserReposFn != nil {
		return m.UserReposFn(username, pageNum, pageSize)
	}

	return domain.Page[domain.RemoteRepo]{}, nil
}

func (m *RemoteStorage) RepoExists(
	owner string,
	repo string,
) (bool, error) {
	m.RepoExistsCalled = true
	m.RepoExistsOwner = owner
	m.RepoExistsRepo = repo

	if m.RepoExistsFn != nil {
		return m.RepoExistsFn(owner, repo)
	}

	return false, nil
}

type RemoteStorageGetter struct {
	Storage domain.RemoteStorage
	Err     error

	Called bool
	ChatID int64
}

func (m *RemoteStorageGetter) Client(
	ctx context.Context,
	chatID int64,
) (domain.RemoteStorage, error) {
	m.Called = true
	m.ChatID = chatID

	return m.Storage, m.Err
}
