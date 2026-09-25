package vfs

import (
	"context"
	"io/fs"
	"os"
)

// AsActor returns a handle that presents actor on every call, so a caller that
// holds authority once — a platform operator acting for a tenant, a service
// acting for the session it is running — does not restamp it on every context.
// The handle carries exactly what the store allows that actor and nothing more,
// and the actor still reaches the store's callbacks, which is what makes a read
// through it identical to a read with [WithActor] stamped by hand.
func AsActor(files Files, actor Actor) Files {
	if files == nil {
		return nil
	}
	return &actorFiles{inner: files, actor: actor}
}

type actorFiles struct {
	inner Files
	actor Actor
}

var _ Files = (*actorFiles)(nil)

func (f *actorFiles) ReadFile(ctx context.Context, name string) ([]byte, error) {
	return f.inner.ReadFile(f.acting(ctx), name)
}

func (f *actorFiles) WriteFile(ctx context.Context, name string, data []byte) error {
	return f.inner.WriteFile(f.acting(ctx), name, data)
}

func (f *actorFiles) MkdirAll(ctx context.Context, name string) error {
	return f.inner.MkdirAll(f.acting(ctx), name)
}

func (f *actorFiles) Remove(ctx context.Context, name string) error {
	return f.inner.Remove(f.acting(ctx), name)
}

func (f *actorFiles) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	return f.inner.Stat(f.acting(ctx), name)
}

func (f *actorFiles) ReadDir(ctx context.Context, name string) ([]os.DirEntry, error) {
	return f.inner.ReadDir(f.acting(ctx), name)
}

func (f *actorFiles) WalkDir(ctx context.Context, name string, fn fs.WalkDirFunc) error {
	return f.inner.WalkDir(f.acting(ctx), name, fn)
}

func (f *actorFiles) Sub(dir string) (Files, error) {
	sub, err := f.inner.Sub(dir)
	if err != nil {
		return nil, err
	}
	return AsActor(sub, f.actor), nil
}

func (f *actorFiles) acting(ctx context.Context) context.Context {
	return WithActor(ctx, f.actor)
}
