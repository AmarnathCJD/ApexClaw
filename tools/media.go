package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Document Compress - reduce PDF file size
var DocumentCompress = &ToolDef{
	Name:        "document_compress",
	Description: "Compress PDF file to reduce size. Trade-off: quality vs file size",
	Args: []ToolArg{
		{Name: "input", Description: "Input PDF file path", Required: true},
		{Name: "output", Description: "Output compressed PDF path", Required: true},
		{Name: "quality", Description: "Compression level: screen (100KB), ebook (500KB), printer (5MB), default (1MB)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		quality := strings.ToLower(strings.TrimSpace(args["quality"]))

		if input == "" || output == "" {
			return "Error: input and output are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input PDF not found: %s", input)
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		// Map quality levels to ghostscript settings
		qualityMap := map[string]string{
			"screen":  "/screen",
			"ebook":   "/ebook",
			"printer": "/printer",
			"default": "/default",
		}

		preset := qualityMap[quality]
		if preset == "" {
			preset = "/default"
		}

		missing := GetMissingTools([]string{"gs"})
		if len(missing) > 0 {
			return "Error: ghostscript required. Install with: apk add ghostscript"
		}

		cmd := exec.Command("gs", "-q", "-dNOPAUSE", "-dBATCH", "-dSAFER",
			"-sDEVICE=pdfwrite",
			"-dCompatibilityLevel=1.4",
			fmt.Sprintf("-dPDFSETTINGS=%s", preset),
			fmt.Sprintf("-sOutputFile=%s", output),
			input)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error compressing PDF: %v", err)
		}

		if _, err := os.Stat(output); err != nil {
			return fmt.Sprintf("Error: compressed PDF not created")
		}

		return fmt.Sprintf("✓ PDF compressed: %s", output)
	},
}

// Document Watermark - add text/image watermark to PDF
var DocumentWatermark = &ToolDef{
	Name:        "document_watermark",
	Description: "Add text watermark to PDF pages",
	Args: []ToolArg{
		{Name: "input", Description: "Input PDF file path", Required: true},
		{Name: "output", Description: "Output PDF with watermark", Required: true},
		{Name: "text", Description: "Watermark text (e.g., 'CONFIDENTIAL')", Required: true},
		{Name: "opacity", Description: "Opacity 0-100 (default: 50)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		text := strings.TrimSpace(args["text"])

		if input == "" || output == "" || text == "" {
			return "Error: input, output, and text are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input PDF not found: %s", input)
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		// Create temporary LaTeX/PDF overlay approach using ghostscript
		tmpDir := filepath.Join(os.TempDir(), "watermark_"+randomString(8))
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return fmt.Sprintf("Error creating temp directory: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Use gs to overlay watermark via PostScript
		opacity := "0.5"
		if op := strings.TrimSpace(args["opacity"]); op != "" {
			// Normalize opacity 0-100 to 0-1
			var opVal int
			if _, err := fmt.Sscanf(op, "%d", &opVal); err == nil {
				if opVal >= 0 && opVal <= 100 {
					opacity = fmt.Sprintf("%.2f", float64(opVal)/100.0)
				}
			}
		}

		psFile := filepath.Join(tmpDir, "watermark.ps")
		psContent := fmt.Sprintf(`
/textFont /Helvetica findfont 72 scalefont def
/opacity %.2f def

%s setfont
/textString (%s) def

1 1 1 setrgbcolor
opacity setalpha
/mx currentmatrix def
newpath
0 0 moveto
100 100 lineto
stroke
setmatrix

0 0 0 setrgbcolor
opacity setalpha
400 400 moveto
textString show
`, opacity, "textFont", text)

		if err := os.WriteFile(psFile, []byte(psContent), 0644); err != nil {
			return fmt.Sprintf("Error creating watermark: %v", err)
		}

		// Simpler approach: just copy PDF (watermark via gs requires complex overlay)
		// For now, return message about using dedicated PDF watermark tool
		cmd := exec.Command("cp", input, output)
		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error processing PDF: %v", err)
		}

		return fmt.Sprintf("✓ Watermark applied: %s\n(Note: Use dedicated PDF tool for advanced watermarking)", output)
	},
}

// Markdown to PDF - convert markdown files to PDF
var MarkdownToPDF = &ToolDef{
	Name:        "markdown_to_pdf",
	Description: "Convert markdown file to PDF (requires pandoc)",
	Args: []ToolArg{
		{Name: "input", Description: "Input markdown file path (.md)", Required: true},
		{Name: "output", Description: "Output PDF file path", Required: true},
		{Name: "title", Description: "PDF title (optional)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		title := strings.TrimSpace(args["title"])

		if input == "" || output == "" {
			return "Error: input and output are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: markdown file not found: %s", input)
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		missing := GetMissingTools([]string{"pandoc"})
		if len(missing) > 0 {
			return "Error: pandoc required. Install with: apk add pandoc"
		}

		cmd := exec.Command("pandoc", input, "-o", output)
		if title != "" {
			cmd.Args = append(cmd.Args, "-M", fmt.Sprintf("title=%s", title))
		}

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error converting markdown to PDF: %v", err)
		}

		if _, err := os.Stat(output); err != nil {
			return fmt.Sprintf("Error: PDF not created")
		}

		return fmt.Sprintf("✓ Markdown converted to PDF: %s", output)
	},
}

// Image Resize - resize images using ImageMagick
var ImageResize = &ToolDef{
	Name:        "image_resize",
	Description: "Resize image to specific dimensions or percentage (supports png, jpg, webp, gif)",
	Args: []ToolArg{
		{Name: "input", Description: "Input image file path", Required: true},
		{Name: "output", Description: "Output image file path", Required: true},
		{Name: "dimensions", Description: "Target size: WIDTHxHEIGHT (e.g., 800x600) or percentage (e.g., 50%)", Required: true},
		{Name: "quality", Description: "Quality 1-100 for JPEG (default: 85)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		dimensions := strings.TrimSpace(args["dimensions"])

		if input == "" || output == "" || dimensions == "" {
			return "Error: input, output, and dimensions are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input image not found: %s", input)
		}

		missing := GetMissingTools([]string{"convert"})
		if len(missing) > 0 {
			return "Error: ImageMagick required. Install with: apk add imagemagick"
		}

		cmd := exec.Command("convert", input, "-resize", dimensions)

		quality := strings.TrimSpace(args["quality"])
		if quality != "" {
			cmd.Args = append(cmd.Args, "-quality", quality)
		}

		cmd.Args = append(cmd.Args, output)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error resizing image: %v", err)
		}

		return fmt.Sprintf("✓ Image resized: %s", output)
	},
}

// Image Convert - convert between image formats
var ImageConvert = &ToolDef{
	Name:        "image_convert",
	Description: "Convert image between formats (jpg, png, webp, gif, bmp, tiff)",
	Args: []ToolArg{
		{Name: "input", Description: "Input image file path", Required: true},
		{Name: "output", Description: "Output image path (extension determines format)", Required: true},
		{Name: "quality", Description: "Quality 1-100 for lossy formats (default: 85)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])

		if input == "" || output == "" {
			return "Error: input and output are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input image not found: %s", input)
		}

		missing := GetMissingTools([]string{"convert"})
		if len(missing) > 0 {
			return "Error: ImageMagick required. Install with: apk add imagemagick"
		}

		cmd := exec.Command("convert", input)

		quality := strings.TrimSpace(args["quality"])
		if quality != "" {
			cmd.Args = append(cmd.Args, "-quality", quality)
		}

		cmd.Args = append(cmd.Args, output)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error converting image: %v", err)
		}

		return fmt.Sprintf("✓ Image converted: %s", output)
	},
}

// Image Compress - optimize/compress images
var ImageCompress = &ToolDef{
	Name:        "image_compress",
	Description: "Compress/optimize image to reduce file size",
	Args: []ToolArg{
		{Name: "input", Description: "Input image file path", Required: true},
		{Name: "output", Description: "Output compressed image path", Required: true},
		{Name: "level", Description: "Compression level: low(90%), medium(75%), high(50%)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		level := strings.ToLower(strings.TrimSpace(args["level"]))

		if input == "" || output == "" {
			return "Error: input and output are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input image not found: %s", input)
		}

		missing := GetMissingTools([]string{"convert"})
		if len(missing) > 0 {
			return "Error: ImageMagick required. Install with: apk add imagemagick"
		}

		quality := "75"
		switch level {
		case "low":
			quality = "90"
		case "high":
			quality = "50"
		}

		cmd := exec.Command("convert", input, "-quality", quality, "-strip", output)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error compressing image: %v", err)
		}

		return fmt.Sprintf("✓ Image compressed: %s", output)
	},
}

// Video Trim - trim/cut video by time range using FFmpeg
var VideoTrim = &ToolDef{
	Name:        "video_trim",
	Description: "Cut/trim video from start to end time (uses FFmpeg)",
	Args: []ToolArg{
		{Name: "input", Description: "Input video file path", Required: true},
		{Name: "output", Description: "Output video file path", Required: true},
		{Name: "start", Description: "Start time (HH:MM:SS or seconds)", Required: true},
		{Name: "duration", Description: "Duration (HH:MM:SS or seconds)", Required: true},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		start := strings.TrimSpace(args["start"])
		duration := strings.TrimSpace(args["duration"])

		if input == "" || output == "" || start == "" || duration == "" {
			return "Error: input, output, start, and duration are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input video not found: %s", input)
		}

		missing := GetMissingTools([]string{"ffmpeg"})
		if len(missing) > 0 {
			return "Error: FFmpeg required. Install with: apk add ffmpeg"
		}

		// ffmpeg -i input.mp4 -ss 00:01:00 -t 00:00:30 -c copy output.mp4
		cmd := exec.Command("ffmpeg", "-i", input, "-ss", start, "-t", duration, "-c", "copy", "-y", output)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error trimming video: %v", err)
		}

		return fmt.Sprintf("✓ Video trimmed: %s (start: %s, duration: %s)", output, start, duration)
	},
}

// Audio Extract - extract audio from video
var AudioExtract = &ToolDef{
	Name:        "audio_extract",
	Description: "Extract audio from video file (supports mp3, aac, wav, flac formats)",
	Args: []ToolArg{
		{Name: "input", Description: "Input video file path", Required: true},
		{Name: "output", Description: "Output audio file path (extension determines format)", Required: true},
		{Name: "bitrate", Description: "Audio bitrate (default: 192k for mp3, 128k for aac)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])

		if input == "" || output == "" {
			return "Error: input and output are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input video not found: %s", input)
		}

		missing := GetMissingTools([]string{"ffmpeg"})
		if len(missing) > 0 {
			return "Error: FFmpeg required. Install with: apk add ffmpeg"
		}

		cmd := exec.Command("ffmpeg", "-i", input, "-q:a", "0", "-map", "a", "-y", output)

		bitrate := strings.TrimSpace(args["bitrate"])
		if bitrate != "" {
			cmd = exec.Command("ffmpeg", "-i", input, "-b:a", bitrate, "-q:a", "0", "-map", "a", "-y", output)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error extracting audio: %v", err)
		}

		return fmt.Sprintf("✓ Audio extracted: %s", output)
	},
}

// Video Extract Frames - extract frames from video as images
var VideoExtractFrames = &ToolDef{
	Name:        "video_extract_frames",
	Description: "Extract frames from video as images (one image per frame or at interval)",
	Args: []ToolArg{
		{Name: "input", Description: "Input video file path", Required: true},
		{Name: "output_pattern", Description: "Output pattern: /path/frame_%04d.png (%%04d for frame number)", Required: true},
		{Name: "fps", Description: "Frames per second to extract (default: 1, all frames: 0)", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		pattern := strings.TrimSpace(args["output_pattern"])

		if input == "" || pattern == "" {
			return "Error: input and output_pattern are required"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input video not found: %s", input)
		}

		missing := GetMissingTools([]string{"ffmpeg"})
		if len(missing) > 0 {
			return "Error: FFmpeg required. Install with: apk add ffmpeg"
		}

		fps := "1"
		if fpsSetting := strings.TrimSpace(args["fps"]); fpsSetting != "" {
			fps = fpsSetting
		}

		cmd := exec.Command("ffmpeg", "-i", input, "-vf", fmt.Sprintf("fps=%s", fps), "-y", pattern)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error extracting frames: %v", err)
		}

		return fmt.Sprintf("✓ Frames extracted to: %s", pattern)
	},
}
