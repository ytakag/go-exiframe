# go-exiframe

| `go-exiframe -f ./sample01.jpeg -black` | `go-exiframe -f ./sample02.jpeg -no-frame` |
| :-------------------------------------: | :----------------------------------------: |
|  ![](./sample/exiframe-sample01.jpeg)   |    ![](./sample/exiframe-sample02.jpeg)    |

## 使い方

```bash
# Install
$ go install github.com/ytakag/go-exiframe@latest

# Help
$ go-exiframe -h
Usage of go-exiframe:

Required:
  -f path
        Image path; repeat -f to process multiple images

Optional — output:
  -o path
        Output JPEG path (single input only)
  -out-dir path
        Output directory (default .)
  -quality int
        JPEG quality (1-100) (default 100)
  -preserve-exif
        Preserve source JPEG EXIF metadata in output

Optional — image layout:
  -pad-to ratio
        Add padding to fit the final image inside this aspect ratio (for example 4:5 or 1.91:1)
  -frame-ratio float
        Frame width as a fraction of the shorter image side (default 0.03)
  -no-frame
        Draw no border around the image
  -black
        Use black frame (default white)
  -overlay-position position
        Overlay position: none, full-width, top-left, top-right, bottom-left, bottom-right (default none)

Optional — displayed information:
  -no-camera
        Omit camera make and model
  -no-lens
        Omit lens make and model
  -no-shooting
        Omit shooting settings (focal length, aperture, exposure, ISO)
  -no-date
        Omit shooting date and time

# Example
$ go-exiframe -f /path/to/image.jpg
# Writes ./exiframe-image.jpg
$ go-exiframe -f /path/to/image.jpg -o /path/to/result.jpg -quality 90
$ go-exiframe -f ./first.jpg -f ./second.jpg -out-dir ./framed -preserve-exif
$ go-exiframe -f ./image.jpg -overlay-position full-width
$ go-exiframe -f ./image.jpg -overlay-position top-right
$ go-exiframe -f ./image.jpg -no-date
$ go-exiframe -f ./image.jpg -no-camera -no-lens
$ go-exiframe -f ./image.jpg -no-shooting -no-date
$ go-exiframe -f ./image.jpg -pad-to 4:5
$ go-exiframe -f ./image.jpg -pad-to 1.91:1 -black
```

## 参考

- [go-exif/v3](https://pkg.go.dev/github.com/dsoprea/go-exif/v3)
- [CIPA DC-008-2024](https://www.cipa.jp/j/std/std-sec.html)
  - デジタルスチルカメラ用画像ファイルフォーマット規格 Exif 3.0
