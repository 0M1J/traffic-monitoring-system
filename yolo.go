package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"runtime"
	"sort"

	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
)

type ModelSession struct {
	Session *ort.AdvancedSession
	Input   *ort.Tensor[float32]
	Output  *ort.Tensor[float32]
}

var (
	modelPath = "./yolov8m.onnx"
	// modelPath   = "./yolov7-tiny.onnx"
	yoloClasses = []string{
		"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
		"traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat", "dog", "horse",
		"sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack", "umbrella", "handbag", "tie",
		"suitcase", "frisbee", "skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove",
		"skateboard", "surfboard", "tennis racket", "bottle", "wine glass", "cup", "fork", "knife", "spoon",
		"bowl", "banana", "apple", "sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza", "donut",
		"cake", "chair", "couch", "potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
		"remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink", "refrigerator", "book",
		"clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush",
	}
	probThreshold float32 = 0.5
	iouThreshold          = 0.7
)

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

type BoundingBox struct {
	Label          string
	Confidence     float32
	X1, Y1, X2, Y2 float64
}

func (b *BoundingBox) String() string {
	return fmt.Sprintf("Object %s (confidence %.2f): (%.2f, %.2f), (%.2f, %.2f)", b.Label, b.Confidence, b.X1, b.Y1, b.X2, b.Y2)
}

func loadImageFile(filePath string) (image.Image, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening %s: %w", filePath, err)
	}
	defer f.Close()

	// Read the file into a []byte
	fileData, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Create a bytes.Buffer from the file data
	buffer := bytes.NewBuffer(fileData)
	pic, err := png.Decode(buffer)
	if err != nil {
		return nil, fmt.Errorf("error decoding %s: %w", filePath, err)
	}
	return pic, nil
}

func prepareInput(pic image.Image, dst *ort.Tensor[float32]) error {
	data := dst.GetData()
	channelSize := 640 * 640
	if len(data) < (channelSize * 3) {
		return fmt.Errorf("destination tensor only holds %d floats, needs "+
			"%d (make sure it's the right shape!)", len(data), channelSize*3)
	}
	redChannel := data[0:channelSize]
	greenChannel := data[channelSize : channelSize*2]
	blueChannel := data[channelSize*2 : channelSize*3]

	// Resize the image to 640x640 using Lanczos3 algorithm
	pic = resize.Resize(640, 640, pic, resize.Lanczos3)
	i := 0
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			r, g, b, _ := pic.At(x, y).RGBA()
			redChannel[i] = float32(r>>8) / 255.0
			greenChannel[i] = float32(g>>8) / 255.0
			blueChannel[i] = float32(b>>8) / 255.0
			i++
		}
	}

	return nil
}

func (b *BoundingBox) toRect() image.Rectangle {
	return image.Rect(int(b.X1), int(b.Y1), int(b.X2), int(b.Y2)).Canon()
}

// Returns the area of b in pixels, after converting to an image.Rectangle.
func (b *BoundingBox) rectArea() int {
	size := b.toRect().Size()
	return size.X * size.Y
}

func (b *BoundingBox) intersection(other *BoundingBox) float32 {
	r1 := b.toRect()
	r2 := other.toRect()
	intersected := r1.Intersect(r2).Canon().Size()
	return float32(intersected.X * intersected.Y)
}

func (b *BoundingBox) union(other *BoundingBox) float32 {
	intersectArea := b.intersection(other)
	totalArea := float32(b.rectArea() + other.rectArea())
	return totalArea - intersectArea
}

func (b *BoundingBox) iou(other *BoundingBox) float32 {
	return b.intersection(other) / b.union(other)
}

func processOutput(output []float32, imgWidth, imgHeight int) []BoundingBox {
	boxes := []BoundingBox{}
	for index := 0; index < 8400; index++ {
		classID, prob := detectClass(output, index)
		if prob < probThreshold {
			continue
		}

		label := yoloClasses[classID]
		xc := output[index]
		yc := output[8400+index]
		w := output[2*8400+index]
		h := output[3*8400+index]

		x1 := int((xc - (w / 2)) * float32(imgWidth))
		y1 := int((yc - (h / 2)) * float32(imgHeight))
		x2 := int((xc + (w / 2)) * float32(imgWidth))
		y2 := int((yc + (h / 2)) * float32(imgHeight))

		boxes = append(boxes, BoundingBox{Label: label, Confidence: prob, X1: float64(x1), Y1: float64(y1), X2: float64(x2), Y2: float64(y2)})
	}
	return filterBoxes(boxes)
}

func detectClass(output []float32, index int) (int, float32) {
	var probability float32 = -1e9
	var classID int
	for col := 0; col < 80; col++ {
		currentProb := output[8400*(col+4)+index]
		if currentProb > probability {
			probability = currentProb
			classID = col
		}
	}
	return classID, probability
}

func filterBoxes(boundingBoxes []BoundingBox) []BoundingBox {
	// Sort the bounding boxes by probability
	sort.Slice(boundingBoxes, func(i, j int) bool {
		return boundingBoxes[i].Confidence < boundingBoxes[j].Confidence
	})

	// Define a slice to hold the final result
	mergedResults := make([]BoundingBox, 0, len(boundingBoxes))

	// Iterate through sorted bounding boxes, removing overlaps
	for _, candidateBox := range boundingBoxes {
		overlapsExistingBox := false
		for _, existingBox := range mergedResults {
			if (&candidateBox).iou(&existingBox) > float32(iouThreshold) {
				overlapsExistingBox = true
				break
			}
		}
		if !overlapsExistingBox {
			mergedResults = append(mergedResults, candidateBox)
		}
	}

	// This will still be in sorted order by confidence
	return mergedResults
}

func RunModel(session *ModelSession, filePath string) error {
	img, err := loadImageFile(filePath)
	if err != nil {
		return err
	}

	if err := prepareInput(img, session.Input); err != nil {
		return err
	}

	// Run the model
	if err := session.Session.Run(); err != nil {
		return fmt.Errorf("error running model: %w", err)
	}

	outputData := session.Output.GetData()
	boxes := processOutput(outputData, img.Bounds().Dx(), img.Bounds().Dy())

	for _, box := range boxes {
		fmt.Println(box)
	}
	return nil
}
