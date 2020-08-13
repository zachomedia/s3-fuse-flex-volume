package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"syscall"
)

func makeResponse(status, message string) map[string]interface{} {
	return map[string]interface{}{
		"status":  status,
		"message": message,
	}
}

/// Return status
func Init() interface{} {
	resp := makeResponse("Success", "No Initialization required")
	resp["capabilities"] = map[string]interface{}{
		"attach":         false,
		"selinuxRelabel": false,
	}
	return resp
}

func isMountPoint(path string) bool {
	cmd := exec.Command("mountpoint", path)
	err := cmd.Run()
	if err != nil {
		return false
	}
	return true
}

func pidFilename(target string) string {
	f256 := sha256.Sum256([]byte(target))
	return hex.EncodeToString(f256[0:])
}

/// If goofys hasn't been mounted yet, mount!
/// If mounted, bind mount to appropriate place.
func Mount(target string, options map[string]string) interface{} {
	bucket := options["bucket"]
	// subPath := options["subPath"]
	dirMode, ok := options["dirMode"]
	if !ok {
		dirMode = "0755"
	}
	fileMode, ok := options["fileMode"]
	if !ok {
		fileMode = "0644"
	}

	args := []string{
		"-o", "allow_other",
		"--dir-mode", dirMode,
		"--file-mode", fileMode,
	}

	if endpoint, ok := options["endpoint"]; ok {
		args = append(args, "--endpoint", endpoint)
	}
	if region, ok := options["region"]; ok {
		args = append(args, "--region", region)
	}
	if uid, ok := options["uid"]; ok {
		args = append(args, "--uid", uid)
	}
	if gid, ok := options["gid"]; ok {
		args = append(args, "--gid", gid)
	}

	debug_s3, ok := options["debug_s3"]
	if ok && debug_s3 == "true" {
		args = append(args, "--debug_s3")
	}

	// Write the pid on the filesystem
	filename := pidFilename(target)

	err := os.MkdirAll(path.Join(os.TempDir(), "goofys"), 0755)
	if err != nil {
		log.Println(err)
	}

	args = append(args, "--pid-file", path.Join(os.TempDir(), "goofys", filename))

	// mountPath := path.Join("/mnt/goofys", bucket)
	mountPath := target

	args = append(args, bucket, mountPath)

	if !isMountPoint(mountPath) {
		exec.Command("umount", mountPath).Run()
		exec.Command("rm", "-rf", mountPath).Run()
		os.MkdirAll(mountPath, 0755)

		mountCmd := exec.Command("goofys", args...)
		mountCmd.Env = os.Environ()
		if accessKey, ok := options["access-key"]; ok {
			mountCmd.Env = append(mountCmd.Env, "AWS_ACCESS_KEY_ID="+accessKey)
		}
		if secretKey, ok := options["secret-key"]; ok {
			mountCmd.Env = append(mountCmd.Env, "AWS_SECRET_ACCESS_KEY="+secretKey)
		}
		var stderr bytes.Buffer
		mountCmd.Stderr = &stderr
		err := mountCmd.Run()
		if err != nil {
			errMsg := err.Error() + ": " + stderr.String()
			if debug_s3 == "true" {
				errMsg += fmt.Sprintf("; /var/log/syslog follows")
				grepCmd := exec.Command("sh", "-c", "grep goofys /var/log/syslog | tail")
				var stdout bytes.Buffer
				grepCmd.Stdout = &stdout
				grepCmd.Run()
				errMsg += stdout.String()
			}
			return makeResponse("Failure", errMsg)
		}
	}

	// srcPath := path.Join(mountPath, subPath)

	// // Create subpath if it does not exist
	// intDirMode, _ := strconv.ParseUint(dirMode, 8, 32)
	// os.MkdirAll(srcPath, os.FileMode(intDirMode))

	// // Now we rmdir the target, and then make a symlink to it!
	// err := os.Remove(target)
	// if err != nil {
	// 	return makeResponse("Failure", err.Error())
	// }

	// err = os.Symlink(srcPath, target)

	return makeResponse("Success", "Mount completed!")
}

func Unmount(target string) interface{} {
	pidfile := pidFilename(target)
	pidstr, err := ioutil.ReadFile(path.Join(os.TempDir(), "goofys", pidfile))
	if err != nil {
		return makeResponse("Failure", fmt.Sprintf("could not load pid file for path %s: %v", target, err.Error()))
	}

	// Terminate the process
	pid, err := strconv.Atoi(string(pidstr))
	if err != nil {
		return makeResponse("Failure", err.Error())
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return makeResponse("Failure", err.Error())
	}
	err = proc.Signal(syscall.SIGTERM)
	if err != nil {
		return makeResponse("Failure", err.Error())
	}

	err = os.Remove(target)
	if err != nil {
		return makeResponse("Failure", err.Error())
	}

	// Remove the pid file
	_ = os.Remove(path.Join(os.TempDir(), "goofys", pidfile))

	return makeResponse("Success", "Successfully unmounted")
}

func Test(target string) interface{} {
	return map[string]interface{}{
		"target":   target,
		"filename": pidFilename(target),
	}
}

func printJSON(data interface{}) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s", string(jsonBytes))
}

func main() {
	switch action := os.Args[1]; action {
	case "init":
		printJSON(Init())
	case "mount":
		optsString := os.Args[3]
		opts := make(map[string]string)
		json.Unmarshal([]byte(optsString), &opts)
		printJSON(Mount(os.Args[2], opts))
	case "unmount":
		printJSON(Unmount(os.Args[2]))
	case "test":
		printJSON(Test(os.Args[2]))
	default:
		printJSON(makeResponse("Not supported", fmt.Sprintf("Operation %s is not supported", action)))
	}

}
