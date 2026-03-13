package playground_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/contenox/contenox/playground"
	"github.com/contenox/contenox/vfsservice"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystem_VFSService(t *testing.T) {
	ctx := t.Context()

	p := playground.New()
	svc, err := p.WithPostgresTestContainer(ctx).GetVFSService(ctx)
	require.NoError(t, err)
	defer p.CleanUp()

	t.Run("CreateFile", func(t *testing.T) {
		testFile := &vfsservice.File{
			Name:        "test.txt",
			ContentType: "text/plain",
			Data:        []byte("test data"),
		}

		createdFile, err := svc.CreateFile(ctx, testFile)
		require.NoError(t, err)
		require.NotEmpty(t, createdFile.ID, "ID should be generated")
		assert.Equal(t, "test.txt", createdFile.Path)
		assert.Equal(t, "text/plain", createdFile.ContentType)
		assert.Equal(t, int64(len(testFile.Data)), createdFile.Size)
		assert.Empty(t, createdFile.ParentID)

		retrieved, err := svc.GetFileByID(ctx, createdFile.ID)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(retrieved.Data, testFile.Data))
	})

	t.Run("UpdateFile", func(t *testing.T) {
		created, err := svc.CreateFile(ctx, &vfsservice.File{
			Name:        "update.txt",
			ContentType: "text/plain",
			Data:        []byte("original data"),
		})
		require.NoError(t, err)
		assert.Equal(t, "update.txt", created.Path)

		newData := []byte("updated data")
		updated, err := svc.UpdateFile(ctx, &vfsservice.File{
			ID:          created.ID,
			ContentType: "text/plain",
			Data:        newData,
		})
		require.NoError(t, err)
		assert.Equal(t, "update.txt", updated.Path)
		assert.True(t, bytes.Equal(updated.Data, newData))
		assert.Equal(t, int64(len(newData)), updated.Size)

		retrieved, err := svc.GetFileByID(ctx, created.ID)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(retrieved.Data, newData))
	})

	t.Run("DeleteFile", func(t *testing.T) {
		fileName := "delete_" + uuid.NewString()[:4] + ".txt"
		created, err := svc.CreateFile(ctx, &vfsservice.File{
			Name:        fileName,
			ContentType: "text/plain",
			Data:        []byte("data to delete"),
		})
		require.NoError(t, err)

		require.NoError(t, svc.DeleteFile(ctx, created.ID))

		_, err = svc.GetFileByID(ctx, created.ID)
		assert.Error(t, err, "should return error for deleted file")
	})

	t.Run("CreateFolder_AtRoot", func(t *testing.T) {
		name := "test_folder_at_root_" + uuid.NewString()[:4]
		folder, err := svc.CreateFolder(ctx, "", name)
		require.NoError(t, err)
		require.NotEmpty(t, folder.ID)
		assert.Equal(t, name, folder.Path)
		assert.Empty(t, folder.ParentID)
	})

	t.Run("RenameFile", func(t *testing.T) {
		oldName := "oldname_" + uuid.NewString()[:4] + ".txt"
		created, err := svc.CreateFile(ctx, &vfsservice.File{
			Name:        oldName,
			ContentType: "text/plain",
			Data:        []byte("data"),
		})
		require.NoError(t, err)

		newName := "newname_" + uuid.NewString()[:4] + ".txt"
		renamed, err := svc.RenameFile(ctx, created.ID, newName)
		require.NoError(t, err)
		assert.Equal(t, newName, renamed.Path)

		retrieved, err := svc.GetFileByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, newName, retrieved.Path)
	})

	t.Run("CreateAndGetFolder", func(t *testing.T) {
		name := "folder1_" + uuid.NewString()[:4]
		folder, err := svc.CreateFolder(ctx, "", name)
		require.NoError(t, err)

		stored, err := svc.GetFolderByID(ctx, folder.ID)
		require.NoError(t, err)
		assert.Equal(t, folder.ID, stored.ID)
		assert.Equal(t, name, stored.Path)
	})

	t.Run("CreateMiniTree", func(t *testing.T) {
		folder1Name := "folder1_minitree_" + uuid.NewString()[:4]
		folder1, err := svc.CreateFolder(ctx, "", folder1Name)
		require.NoError(t, err)

		retrieved1, err := svc.GetFolderByID(ctx, folder1.ID)
		require.NoError(t, err)
		assert.Equal(t, folder1Name, retrieved1.Path)
		assert.Empty(t, retrieved1.ParentID)

		folder2Name := "folder2"
		folder2, err := svc.CreateFolder(ctx, folder1.ID, folder2Name)
		require.NoError(t, err)

		stored2, err := svc.GetFolderByID(ctx, folder2.ID)
		require.NoError(t, err)
		assert.Equal(t, folder1Name+"/"+folder2Name, stored2.Path)
		assert.Equal(t, folder1.ID, stored2.ParentID)
	})

	t.Run("RenameFolder", func(t *testing.T) {
		oldFolderName := "old_folder_rename_test_" + uuid.NewString()[:4]
		folder, err := svc.CreateFolder(ctx, "", oldFolderName)
		require.NoError(t, err)

		created1, err := svc.CreateFile(ctx, &vfsservice.File{
			Name:        "file1.txt",
			ParentID:    folder.ID,
			ContentType: "text/plain",
			Data:        []byte("data1"),
		})
		require.NoError(t, err)

		subFolder, err := svc.CreateFolder(ctx, folder.ID, "sub")
		require.NoError(t, err)

		created2, err := svc.CreateFile(ctx, &vfsservice.File{
			Name:        "file2.txt",
			ParentID:    subFolder.ID,
			ContentType: "text/plain",
			Data:        []byte("data2"),
		})
		require.NoError(t, err)

		newFolderName := "new_folder_renamed_" + uuid.NewString()[:4]
		renamed, err := svc.RenameFolder(ctx, folder.ID, newFolderName)
		require.NoError(t, err)
		assert.Equal(t, newFolderName, renamed.Path)
		assert.Empty(t, renamed.ParentID)

		// Child paths update via path reconstruction on read
		retrievedFile1, err := svc.GetFileByID(ctx, created1.ID)
		require.NoError(t, err)
		assert.Equal(t, newFolderName+"/file1.txt", retrievedFile1.Path)

		retrievedSub, err := svc.GetFolderByID(ctx, subFolder.ID)
		require.NoError(t, err)
		assert.Equal(t, newFolderName+"/sub", retrievedSub.Path)

		retrievedFile2, err := svc.GetFileByID(ctx, created2.ID)
		require.NoError(t, err)
		assert.Equal(t, newFolderName+"/sub/file2.txt", retrievedFile2.Path)
	})

	t.Run("ListAllPaths", func(t *testing.T) {
		base := "listtest_" + uuid.NewString()[:4]
		folder1Name := base + "_folder1"
		rootFile1Name := base + "_rootfile1.txt"
		rootFile2Name := base + "_rootfile2.txt"

		folder1, err := svc.CreateFolder(ctx, "", folder1Name)
		require.NoError(t, err)

		folder2, err := svc.CreateFolder(ctx, folder1.ID, "folder2_in_list")
		require.NoError(t, err)

		_, err = svc.CreateFile(ctx, &vfsservice.File{Name: "file1_in_list.txt", ParentID: folder2.ID, ContentType: "text/plain", Data: []byte("data1")})
		require.NoError(t, err)

		_, err = svc.CreateFile(ctx, &vfsservice.File{Name: "file2_in_folder1.txt", ParentID: folder1.ID, ContentType: "text/plain", Data: []byte("data")})
		require.NoError(t, err)

		_, err = svc.CreateFile(ctx, &vfsservice.File{Name: rootFile1Name, ContentType: "text/plain", Data: []byte("root1")})
		require.NoError(t, err)

		_, err = svc.CreateFile(ctx, &vfsservice.File{Name: rootFile2Name, ContentType: "text/plain", Data: []byte("root2")})
		require.NoError(t, err)

		rootFiles, err := svc.GetFilesByPath(ctx, "")
		require.NoError(t, err)

		expectedRoot := map[string]bool{folder1Name: true, rootFile1Name: true, rootFile2Name: true}
		found := 0
		for _, f := range rootFiles {
			if expectedRoot[f.Path] {
				found++
			}
		}
		assert.Equal(t, len(expectedRoot), found, "root listing missing expected items: got %v", filePaths(rootFiles))

		filesInFolder1, err := svc.GetFilesByPath(ctx, folder1Name)
		require.NoError(t, err)
		expectedInFolder1 := map[string]bool{
			folder1Name + "/folder2_in_list":      true,
			folder1Name + "/file2_in_folder1.txt": true,
		}
		assert.Len(t, filesInFolder1, len(expectedInFolder1))
		for _, f := range filesInFolder1 {
			assert.True(t, expectedInFolder1[f.Path], "unexpected item in folder1: %s", f.Path)
		}
	})

	t.Run("MoveFile_Simple_RootToSubfolder", func(t *testing.T) {
		folderName := "move_target_folder_" + uuid.NewString()[:4]
		targetFolder, err := svc.CreateFolder(ctx, "", folderName)
		require.NoError(t, err)

		fileName := "move_me_" + uuid.NewString()[:4] + ".txt"
		fileToMove, err := svc.CreateFile(ctx, &vfsservice.File{Name: fileName, ContentType: "text/plain", Data: []byte("move data")})
		require.NoError(t, err)
		assert.Empty(t, fileToMove.ParentID)

		moved, err := svc.MoveFile(ctx, fileToMove.ID, targetFolder.ID)
		require.NoError(t, err)
		assert.Equal(t, targetFolder.ID, moved.ParentID)
		assert.Equal(t, folderName+"/"+fileName, moved.Path)

		retrieved, err := svc.GetFileByID(ctx, fileToMove.ID)
		require.NoError(t, err)
		assert.Equal(t, targetFolder.ID, retrieved.ParentID)
		assert.Equal(t, folderName+"/"+fileName, retrieved.Path)
	})

	t.Run("MoveFile_ToRoot", func(t *testing.T) {
		sourceFolderName := "move_source_folder_" + uuid.NewString()[:4]
		sourceFolder, err := svc.CreateFolder(ctx, "", sourceFolderName)
		require.NoError(t, err)

		fileName := "move_me_to_root_" + uuid.NewString()[:4] + ".txt"
		fileToMove, err := svc.CreateFile(ctx, &vfsservice.File{Name: fileName, ParentID: sourceFolder.ID, ContentType: "text/plain", Data: []byte("to root")})
		require.NoError(t, err)
		assert.Equal(t, sourceFolderName+"/"+fileName, fileToMove.Path)

		moved, err := svc.MoveFile(ctx, fileToMove.ID, "")
		require.NoError(t, err)
		assert.Empty(t, moved.ParentID)
		assert.Equal(t, fileName, moved.Path)
	})

	t.Run("MoveFile_NameCollision", func(t *testing.T) {
		folderName := "move_collision_folder_" + uuid.NewString()[:4]
		targetFolder, err := svc.CreateFolder(ctx, "", folderName)
		require.NoError(t, err)

		collidingName := "iamhere_" + uuid.NewString()[:4] + ".txt"
		_, err = svc.CreateFile(ctx, &vfsservice.File{Name: collidingName, ParentID: targetFolder.ID, ContentType: "text/plain", Data: []byte("existing")})
		require.NoError(t, err)

		fileToMove, err := svc.CreateFile(ctx, &vfsservice.File{Name: collidingName, ContentType: "text/plain", Data: []byte("original")})
		require.NoError(t, err)

		_, err = svc.MoveFile(ctx, fileToMove.ID, targetFolder.ID)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "unique constraint"),
			"expected name collision error, got: %v", err)
	})

	t.Run("MoveFolder_Simple_RootToSubfolder", func(t *testing.T) {
		parentName := "move_parent_F_" + uuid.NewString()[:4]
		targetParent, err := svc.CreateFolder(ctx, "", parentName)
		require.NoError(t, err)

		folderToMoveName := "folder_to_move_" + uuid.NewString()[:4]
		folderToMove, err := svc.CreateFolder(ctx, "", folderToMoveName)
		require.NoError(t, err)

		childName := "child_" + uuid.NewString()[:4] + ".txt"
		child, err := svc.CreateFile(ctx, &vfsservice.File{Name: childName, ParentID: folderToMove.ID, ContentType: "text/plain", Data: []byte("child")})
		require.NoError(t, err)

		moved, err := svc.MoveFolder(ctx, folderToMove.ID, targetParent.ID)
		require.NoError(t, err)
		assert.Equal(t, targetParent.ID, moved.ParentID)
		assert.Equal(t, parentName+"/"+folderToMoveName, moved.Path)

		retrievedChild, err := svc.GetFileByID(ctx, child.ID)
		require.NoError(t, err)
		assert.Equal(t, parentName+"/"+folderToMoveName+"/"+childName, retrievedChild.Path)
		assert.Equal(t, moved.ID, retrievedChild.ParentID)
	})

	t.Run("MoveFolder_ToRoot", func(t *testing.T) {
		sourceName := "move_F_source_" + uuid.NewString()[:4]
		source, err := svc.CreateFolder(ctx, "", sourceName)
		require.NoError(t, err)

		folderToMoveName := "move_me_F_to_root_" + uuid.NewString()[:4]
		folderToMove, err := svc.CreateFolder(ctx, source.ID, folderToMoveName)
		require.NoError(t, err)

		moved, err := svc.MoveFolder(ctx, folderToMove.ID, "")
		require.NoError(t, err)
		assert.Empty(t, moved.ParentID)
		assert.Equal(t, folderToMoveName, moved.Path)
	})
}

func filePaths(files []vfsservice.File) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths
}
