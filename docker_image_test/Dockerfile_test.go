package docker_image_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	dockerCmdExecutable = "docker"
	imageName           = "otel-desktop-viewer"
	imageTag            = "ubuntu-25.04"

	containerName    = "otel-desktop-viewer-test-container"
	containerWebPort = "8000"
	hostWebPort      = "8001"

	healthCheckUrl = "http://localhost:" + hostWebPort
)

func TestDockerfileBuildAndExecution(t *testing.T) {
	// check docker command is available

	if _, err := exec.LookPath(dockerCmdExecutable); err != nil {
		t.Skipf("Docker command not found in PATH. Skipping integration test: %v", err)
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed to get current file path for docker context.")
	}
	dockerBuildContext := filepath.Join(filepath.Dir(filepath.Dir(filename)))

	checkAndStopContainer(t)

	// remove the docker image
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageTag)
	t.Logf("CLEANUP: Removing Docker image '%s'...", fullImageName)
	// Use -f (force) to ensure removal, even if it was used by a stopped container.
	rmImageCmd := exec.Command(dockerCmdExecutable, "rmi", "-f", fullImageName)
	if output, err := rmImageCmd.CombinedOutput(); err != nil {
		t.Logf("CLEANUP: Failed to remove Docker image '%s': %v\nOutput: %s", fullImageName, err, string(output))
	} else {
		t.Logf("CLEANUP: Docker image '%s' removed.", fullImageName)
	}

	// build docker image
	t.Logf("PHASE 1/4: Building Docker image '%s:%s' from context '%s'...", imageName, imageTag, dockerBuildContext)
	buildCmd := exec.Command(dockerCmdExecutable, "build", "-t", fmt.Sprintf("%s:%s", imageName, imageTag), dockerBuildContext)

	var buildStdout, buildStderr bytes.Buffer
	buildCmd.Stdout = &buildStdout
	buildCmd.Stderr = &buildStderr

	err := buildCmd.Run()

	buildCombinedOutput := fmt.Sprintf("BUILD STDOUT:\n%s\nBUILD STDERR:\n%s",
		buildStdout.String(), buildStderr.String())

	if err != nil {
		t.Fatalf("Docker image build failed: %v\nBUILD OUT:\n%s", err, buildCombinedOutput)
	}
	t.Logf("Image build successful. Build output (STDOUT/STDERR combined):\n%s", buildCombinedOutput)

	// run the built docker container
	t.Logf("PHASE 2/4: Running Docker container '%s' from image '%s:%s' on port %s:%s...", containerName, imageName, imageTag, hostWebPort, containerWebPort)
	runCmd := exec.Command(dockerCmdExecutable, "run",
		"--pull", "never",
		"-d",
		"-p", fmt.Sprintf("%s:%s", hostWebPort, containerWebPort),
		"--name", containerName,
		fmt.Sprintf("%s:%s", imageName, imageTag),
	)

	var runStdout, runStderr bytes.Buffer // Capture output for container ID and errors
	runCmd.Stdout = &runStdout
	runCmd.Stderr = &runStderr

	if errRun := runCmd.Run(); errRun != nil {
		t.Fatalf("Docker container failed to run: %v\nRUN STDOUT:\n%s\nRUN STDERR:\n%s", err, runStdout.String(), runStderr.String())
	}
	containerID := strings.TrimSpace(runStdout.String())
	t.Logf("Container '%s' started with ID: %s", containerName, containerID)

	testUpAndRunning(t)

	t.Logf("PHASE 4/4: Stopping container %s", containerName)
	defer checkAndStopContainer(t)

}

func testUpAndRunning(t *testing.T) {
	fmt.Printf("Starting OTel Desktop Viewer API health check for '%s'...\n", healthCheckUrl)

	success := false
	const maxAttempts = 5
	const retryDelay = 3 * time.Second

	t.Logf("PHASE 3/4: Calling the frontend, max %d times, with %d sec delay", maxAttempts, int(retryDelay/time.Second))
	for i := 1; i <= maxAttempts; i++ {
		fmt.Printf("Attempt %d of %d: Checking API at %s\n", i, maxAttempts, healthCheckUrl)

		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(healthCheckUrl)
		if err != nil {
			fmt.Printf("Error reaching API: %v\n", err)
			time.Sleep(retryDelay)
			continue
		}
		defer resp.Body.Close() // Ensure the response body is closed

		_, _ = io.ReadAll(resp.Body)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Printf("API responded successfully with status code: %d\n", resp.StatusCode)
			success = true
			break
		} else {
			fmt.Printf("API responded with non-2xx status code: %d\n", resp.StatusCode)
			time.Sleep(retryDelay)
		}
	}

	if !success {
		t.Errorf("Test failed: All %d attempts to reach the OTel Desktop Viewer API failed.", maxAttempts)
		fmt.Printf("\nAll %d attempts to reach the OTel Desktop Viewer API failed.\n", maxAttempts)

	} else {
		fmt.Println("\nOTel Desktop Viewer API is reachable. No action needed for the Docker container.")
	}
}

func checkAndStopContainer(t *testing.T) {

	// stop and remove the docker container
	t.Logf("CLEANUP: Stopping and removing container '%s'...", containerName)
	// stop the container first if it's running
	stopCmd := exec.Command(dockerCmdExecutable, "stop", containerName)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		t.Logf("CLEANUP: Failed to stop container '%s' (might not have been running or already stopped): %v\nOutput: %s", containerName, err, string(output))
	} else if strings.TrimSpace(string(output)) == containerName {
		t.Logf("CLEANUP: Container '%s' stopped.", containerName)
	}

	// then remove the container
	rmCmd := exec.Command(dockerCmdExecutable, "rm", containerName)
	if output, err := rmCmd.CombinedOutput(); err != nil {
		t.Logf("CLEANUP: Failed to remove container '%s' (might not exist): %v\nOutput: %s", containerName, err, string(output))
	} else if strings.TrimSpace(string(output)) == containerName {
		t.Logf("CLEANUP: Container '%s' removed.", containerName)
	}

}
