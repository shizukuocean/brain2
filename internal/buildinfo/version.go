package buildinfo

// These defaults preserve the current developer build. Release scripts inject
// immutable product versions with -ldflags -X so the binary, update checks and
// Launcher catalog all report the exact packaged version.
var ManagerVersion = "0.6.6"
var ManagerDisplayVersion = "0.6.6 WIP"
var LauncherVersion = "0.6.6"
var LauncherDisplayVersion = "0.6.6 WIP"
var UpdateManifestURL = ""
