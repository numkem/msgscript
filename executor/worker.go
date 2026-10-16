package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type WorkerInput struct {
	NatsURL     string                  `json:"nats_url"`
	ScriptStore *WorkerInputScriptStore `json:"script_store"`
	LuaExecutor *WorkerInputLuaExecutor `json:"lua_executor"`
	Message     *Message                `json:"messsage"`
}

type WorkerInputScriptStore struct {
	BackendName      string `json:"backend_name"`
	EtcdURL          string `json:"etcd_url"`
	ScriptDirectory  string `json:"script_directory"`
	LibraryDirectory string `json:"library_directory"`
}

type WorkerInputLuaExecutor struct {
	PluginDir string `json:"plugin_dir"`
}

type WorkerOutput struct {
	Error error  `json:"error"`
	Reply *Reply `json:"reply"`
}

func SpawnWorker(ctx context.Context, workerPath string, envPath string, input WorkerInput) (*Reply, error) {
	w := exec.CommandContext(ctx, workerPath)

	if envPath != "" {
		f, err := os.Open(envPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read worker environment file %s: %v", envPath, err)
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			w.Env = append(w.Env, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to scan environment file %s: %v", envPath, err)
		}
	}

	w.Env = append(w.Env, fmt.Sprintf("DEBUG=%s", os.Getenv("DEBUG"))) // Propagate the DEBUG flag
	w.Stderr = os.Stderr

	// Open a pipe to the process' STDIN and STDOUT
	stdin, err := w.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdin for the process")
	}

	stdout, err := w.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout for the process")
	}
	defer stdout.Close()

	err = w.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start worker: %v", err)
	}

	// Read the output of the process
	err = json.NewEncoder(stdin).Encode(input)
	if err != nil {
		return nil, fmt.Errorf("failed to write worker input to buffer: %v", err)
	}
	stdin.Close() // Close it so the worker knows it's got everything

	output := new(WorkerOutput)
	err = json.NewDecoder(stdout).Decode(output)
	if err != nil {
		err = fmt.Errorf("failed to read output from worker: %v", err)
	}

	// Check the worker itself didn't run into error. This doesn't check of errors from the script executed
	if output.Error != nil {
		err = fmt.Errorf("worker ran into error: %v", err)
	}

	err = w.Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to wait for process: %v", err)
	}

	return output.Reply, err
}
