package main

import (
	"fmt"
	"runtime"

	ort "github.com/yalue/onnxruntime_go"
)

type ModelSession struct {
	Session *ort.AdvancedSession
	Input   *ort.Tensor[float32]
	Output  *ort.Tensor[float32]
}

var modelPath = "./yolov8n.onnx"

func getSharedLibPath() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "./onxxrntime/onnxruntime.dll"
		}
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return "./onxxrntime/onnxruntime_arm64.dylib"
		case "amd64":
			return "./onxxrntime/onnxruntime_amd64.dylib"
		}
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "./onxxrntime/onnxruntime_arm64.so"
		}
		return "./onxxrntime/onnxruntime.so"
	}
	panic("Unsupported system configuration for ONNX runtime library")
}

func initSession() (*ModelSession, error) {
	ort.SetSharedLibraryPath(getSharedLibPath())
	err := ort.InitializeEnvironment()
	if err != nil {
		return nil, fmt.Errorf("error initializing ORT environment: %w", err)
	}

	inputShape, outputShape := ort.NewShape(1, 3, 640, 640), ort.NewShape(1, 84, 8400)
	inputTensor, err := ort.NewEmptyTensor[float32](inputShape)
	if err != nil {
		return nil, fmt.Errorf("error creating input tensor: %w", err)
	}
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputTensor.Destroy()
		return nil, fmt.Errorf("error creating output tensor: %w", err)
	}

	options, err := ort.NewSessionOptions()
	if err != nil {
		inputTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("error creating ORT session options: %w", err)
	}
	defer options.Destroy()

	session, err := ort.NewAdvancedSession(modelPath, []string{"images"}, []string{"output0"}, []ort.ArbitraryTensor{inputTensor}, []ort.ArbitraryTensor{outputTensor}, options)
	if err != nil {
		inputTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("error creating ORT session: %w", err)
	}

	return &ModelSession{Session: session, Input: inputTensor, Output: outputTensor}, nil
}

func (m *ModelSession) Destroy() {
	m.Session.Destroy()
	m.Input.Destroy()
	m.Output.Destroy()
}
