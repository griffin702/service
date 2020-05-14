package upload

import (
	"bytes"
	"fmt"
	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"github.com/ulricqin/goutils/filetool"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	StaticDir      string
	MinSize        int64
	MaxSize        int64
	MaxWidthHeight int
	SmallMaxWH     int
	AlbumID        int64
	LastSource     string
	UploadType     int
	W              int
	H              int
}

var (
	uploadTypeMap   = map[int]string{1: "bigpic", 2: "smallpic", 3: "bigsmallpic", 4: "media/mp4", 5: "media/mp3"}
	acceptFileTypes = regexp.MustCompile(`(jpg|gif|p?jpeg|(x-)?png|mp3|mp4)`)
)

var (
	ErrorUploadType = errors.New("Type of Upload not allowed")
	ErrorFileType   = errors.New("Type of file not allowed")
	ErrorSmallSize  = errors.New("File is too small")
	ErrorBigSize    = errors.New("File is too big")
)

type FileInfo struct {
	Config        *Config     `json:"-"`
	Image         image.Image `json:"-"`
	Size          int64       `json:"-"`
	FileType      string      `json:"-"`
	Name          string      `json:"name"`
	URL           string      `json:"url"`
	ScreenShotURL string      `json:"screen_shot_url"`
	DialogID      string      `json:"dialog_id"`
	Success       int         `json:"success"`
	Message       string      `json:"message"`
}

type Sizer interface {
	Size() int64
}

func NewFileInfo(file io.Reader, fileName string, opt ...*Config) (*FileInfo, error) {
	config := &Config{
		StaticDir:      "../static",
		MinSize:        1,
		MaxSize:        10000000,
		MaxWidthHeight: 1280,
		SmallMaxWH:     720,
		UploadType:     1,
	}
	cfg := new(Config)
	if len(opt) > 0 {
		cfg = opt[0]
	}
	if cfg.StaticDir != "" {
		config.StaticDir = cfg.StaticDir
	}
	if cfg.MinSize != 0 {
		config.MinSize = cfg.MinSize
	}
	if cfg.MaxSize != 0 {
		config.MaxSize = cfg.MaxSize
	}
	if cfg.MaxWidthHeight != 0 {
		config.MaxWidthHeight = cfg.MaxWidthHeight
	}
	if cfg.SmallMaxWH != 0 {
		config.SmallMaxWH = cfg.SmallMaxWH
	}
	if cfg.AlbumID != 0 {
		config.AlbumID = cfg.AlbumID
	}
	if cfg.LastSource != "" {
		config.LastSource = cfg.LastSource
	}
	if cfg.UploadType != 0 {
		config.UploadType = cfg.UploadType
	}
	if cfg.W != 0 {
		config.W = cfg.W
	}
	if cfg.H != 0 {
		config.H = cfg.H
	}
	f := &FileInfo{
		Config: config,
		Name:   fileName,
	}
	if cfg.UploadType == 4 || cfg.UploadType == 5 {
		f.ParseMediaExt(fileName)
	} else {
		var err error
		f.Image, f.FileType, err = image.Decode(file)
		if err != nil {
			f.Message = err.Error()
			return f, err
		}
	}
	if err := f.ValidateType(); err != nil {
		f.Message = err.Error()
		return f, err
	}
	if size, ok := file.(Sizer); ok {
		f.Size = size.Size()
	}
	if err := f.ValidateSize(); err != nil {
		return f, err
	}
	return f, nil
}

func (f *FileInfo) ValidateType() (err error) {
	if _, ok := uploadTypeMap[f.Config.UploadType]; !ok {
		return ErrorUploadType
	}
	if !acceptFileTypes.MatchString(f.FileType) {
		return ErrorFileType
	}
	return nil
}

func (f *FileInfo) ValidateSize() (err error) {
	if f.Size < f.Config.MinSize {
		return ErrorSmallSize
	}
	if f.Size > f.Config.MaxSize {
		return ErrorBigSize
	}
	return nil
}

func (f *FileInfo) JoinInfo() (path string) {
	timeNow := time.Now().UnixNano()
	staticDir := f.getDefaultStaticDir(f.Config.StaticDir)
	fileSaveName := fmt.Sprintf("%s/upload/%s/%s",
		staticDir, uploadTypeMap[f.Config.UploadType],
		time.Now().Format("20060102"),
	)
	_ = filetool.InsureDir(fileSaveName)
	if f.Config.UploadType == 4 || f.Config.UploadType == 5 {
		f.ScreenShotURL = fmt.Sprintf("%s/%d.jpg", fileSaveName, timeNow)
	}
	return fmt.Sprintf("%s/%d.%s", fileSaveName, timeNow, f.FileType)
}

func (f *FileInfo) SaveImage(path string) (err error) {
	if f.Config.UploadType == 1 { //上传类型1：文章上传，保存大图小图，默认small限制720px
		if err = f.CreatePicScale(path, 0, 0, 88); err != nil {
			f.Message = err.Error()
			return
		}
		small := f.ChangeToSmall(path)
		mw, mh := f.RetMaxWH(f.Config.SmallMaxWH)
		if err = f.CreatePicScale(small, mw, mh, 88); err != nil {
			f.Message = err.Error()
			return
		}
		//保存成功，则删除旧资源
		if !f.CheckSource() {
			f.RemoveLastSource(f.Config.LastSource)
		}
		f.URL = strings.TrimLeft(small, ".")
		f.Success = 1
		f.Message = "上传成功"
		return
	}
	if f.Config.UploadType == 2 { //上传类型2：头像、封面等上传，只保存小图
		if err = f.CreatePicScale(path, f.Config.W, f.Config.H, 88); err != nil {
			f.Message = err.Error()
			return
		}
		if !f.CheckSource() {
			f.RemoveLastSource(f.Config.LastSource, false)
		}
		f.URL = strings.TrimLeft(path, ".")
		f.Success = 1
		f.Message = "上传成功"
		return
	}
	if f.Config.UploadType == 3 { //上传类型3：照片上传，album=0为心情上传，同时保存大图小图
		if err = f.CreatePicScale(path, 0, 0, 88); err != nil {
			f.Message = err.Error()
			return
		}
		small := f.ChangeToSmall(path)
		if f.Config.AlbumID > 0 {
			if err = f.CreatePicClip(small, f.Config.W, f.Config.H, 88); err != nil {
				f.Message = err.Error()
				return
			}
		} else {
			mw, mh := f.RetMaxWH(f.Config.SmallMaxWH)
			if err = f.CreatePicScale(small, mw, mh, 88); err != nil {
				f.Message = err.Error()
				return
			}
		}
		if !f.CheckSource() {
			f.RemoveLastSource(f.Config.LastSource)
		}
		f.URL = strings.TrimLeft(path, ".")
		f.Success = 1
		f.Message = "上传成功"
		return
	}
	return
}

/*
* 图片裁剪
* 入参:1、file，2、输出路径，3、原图width，4、原图height，5、精度
* 规则:照片生成缩略图尺寸为w * h，先进行缩小，再进行平均裁剪
*
* 返回:error
 */
func (f *FileInfo) CreatePicClip(path string, w, h, q int) error {
	rw, rh := f.RetRealWHEXT()
	x0, x1, y0, y1 := 0, w, 0, h
	sh := rh * w / rw
	sw := rw * h / rh
	if sh > 135 {
		f.Image = resize.Resize(uint(w), uint(sh), f.Image, resize.Lanczos3)
		y0 = (sh - h) / 4
		y1 = y0 + h
	} else {
		f.Image = resize.Resize(uint(sw), uint(h), f.Image, resize.Lanczos3)
		x0 = (sw - w) / 2
		x1 = x0 + w
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	switch f.FileType {
	case "jpeg":
		img := f.Image.(*image.YCbCr)
		subImg := img.SubImage(image.Rect(x0, y0, x1, y1)).(*image.YCbCr)
		return jpeg.Encode(out, subImg, &jpeg.Options{Quality: q})
	case "png":
		switch f.Image.(type) {
		case *image.NRGBA:
			img := f.Image.(*image.NRGBA)
			subImg := img.SubImage(image.Rect(x0, y0, x1, y1)).(*image.NRGBA)
			return png.Encode(out, subImg)
		case *image.RGBA:
			img := f.Image.(*image.RGBA)
			subImg := img.SubImage(image.Rect(x0, y0, x1, y1)).(*image.RGBA)
			return png.Encode(out, subImg)
		}
	case "gif":
		img := f.Image.(*image.Paletted)
		subImg := img.SubImage(image.Rect(x0, y0, x1, y1)).(*image.Paletted)
		return gif.Encode(out, subImg, &gif.Options{})
	default:
		return errors.New("ERROR FORMAT")
	}
	return nil
}

/*
* 缩略图生成
* 入参:1、file，2、输出路径，3、输出width，4、输出height，5、精度
* 规则: width,height是想要生成的尺寸
*
* 返回:error
 */
func (f *FileInfo) CreatePicScale(path string, w, h, q int) error {
	if w == 0 || h == 0 {
		w, h = f.RetRealWHEXT()
		//限制保存原图的长宽最大允许像素
		maxNum := f.Config.MaxWidthHeight
		if w < h && h > maxNum {
			w = w * maxNum / h
			h = maxNum
		} else if w >= h && w > maxNum {
			h = h * maxNum / w
			w = maxNum
		}
	}
	canvas := resize.Resize(uint(w), uint(h), f.Image, resize.Lanczos3)
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	switch f.FileType {
	case "jpeg":
		return jpeg.Encode(out, canvas, &jpeg.Options{Quality: q})
	case "png":
		return png.Encode(out, canvas)
	case "gif":
		return gif.Encode(out, canvas, &gif.Options{})
	default:
		return errors.New("ERROR FORMAT")
	}
}

func (f *FileInfo) ChangeToSmall(path string) string {
	arr1 := strings.Split(path, "/")
	filename := arr1[len(arr1)-1]
	arr2 := strings.Split(filename, ".")
	ext := "." + arr2[len(arr2)-1]
	small := strings.Replace(path, ext, "_small"+ext, 1)
	return small
}

func (f *FileInfo) RetMaxWH(max int) (int, int) {
	w, h := f.RetRealWHEXT()
	var sw, sh int
	if w < h && h > max {
		sh = max
		sw = w * max / h
	} else if w >= h && w > max {
		sw = max
		sh = h * max / w
	} else {
		sw = w
		sh = h
	}
	return sw, sh
}

func (f *FileInfo) RetRealWHEXT() (int, int) {
	w := f.Image.Bounds().Max.X
	h := f.Image.Bounds().Max.Y
	return w, h
}

func (f *FileInfo) GetFrame(mediaPath string) (string, error) {
	cmd := exec.Command("ffmpeg", "-i", mediaPath, "-y", "-f", "image2", "-t", "0.001", f.ScreenShotURL)
	buf := new(bytes.Buffer)
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		f.Success = 0
		f.Message = err.Error()
		return "", err
	}
	f.Success = 1
	f.Message = "上传成功"
	return buf.String(), nil
}

func (f *FileInfo) RemoveLastSource(lastSrc string, small ...bool) {
	removeSmall := true
	if len(small) > 0 {
		removeSmall = small[0]
	}
	_ = os.Remove(".." + lastSrc)
	if removeSmall {
		_ = os.Remove(".." + f.ChangeToSmall(lastSrc))
	}
}

func (f *FileInfo) ParseMediaExt(fileName string) {
	list := strings.Split(f.Name, ".")
	if len(list) > 1 {
		if list[1] != "" {
			f.FileType = list[1]
		}
	}
}

func (f *FileInfo) CheckSource() bool {
	if f.Config.LastSource == "" {
		return true
	}
	var defaultDir = "/upload/default/"
	if index := strings.Index(f.Config.LastSource, defaultDir); index != -1 {
		return true
	}
	return false
}

func (f *FileInfo) getDefaultStaticDir(staticCfg string) (dir string) {
	staticDirList := strings.Split(staticCfg, " ")
	if len(staticDirList) > 0 {
		def := strings.Split(staticDirList[0], ":")
		if len(def) == 2 {
			return def[1]
		}
	}
	return staticCfg
}
