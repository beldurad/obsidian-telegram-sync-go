package mock

import (
	"context"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

type RemoteStorage struct {
	domain.RemoteStorage

	DirectoryFn func(
		owner string,
		repo string,
		path string,
		pageNum int,
		pageSize int,
	) (domain.Page[domain.DirElem], error)

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
}

func (m *RemoteStorage) Directory(
	owner string,
	repo string,
	path string,
	pageNum int,
	pageSize int,
) (domain.Page[domain.DirElem], error) {
	m.DirectoryCalled = true
	m.DirectoryOwner = owner
	m.DirectoryRepo = repo
	m.DirectoryPath = path
	m.DirectoryPage = pageNum
	m.DirectorySize = pageSize

	if m.DirectoryFn != nil {
		return m.DirectoryFn(owner, repo, path, pageNum, pageSize)
	}

	return domain.Page[domain.DirElem]{}, nil
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
