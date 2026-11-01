package remote

import "golang.org/x/sys/windows"

// stillActive is the exit code of a process that has not exited
const stillActive = 259

// processAlive tells whether the process exists and has not exited
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Access denied: the process exists but is not ours
		return err == windows.ERROR_ACCESS_DENIED
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return true
	}
	return code == stillActive
}
