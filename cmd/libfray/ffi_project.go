package main

import "C"

import (
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

//export FrayDiscoverProject
func FrayDiscoverProject(startDir *C.char) *C.char {
	dir := cStringToGo(startDir)
	project, err := core.DiscoverProject(dir)
	if err != nil {
		return returnJSON(errorResponse(err.Error()))
	}
	return returnJSON(successResponse(ProjectResponse{
		Root:   project.Root,
		DBPath: project.DBPath,
	}))
}

//export FrayOpenDatabase
func FrayOpenDatabase(projectPath *C.char) C.ulonglong {
	path := cStringToGo(projectPath)
	project, err := core.DiscoverProject(path)
	if err != nil {
		return 0
	}
	database, err := db.OpenDatabase(project)
	if err != nil {
		return 0
	}
	return C.ulonglong(registerHandle(database, project, path))
}

//export FrayCloseDatabase
func FrayCloseDatabase(handle C.ulonglong) {
	closeHandle(uint64(handle))
}
