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
  -black
        Use black color frame (default white)
  -f string
        Image path; repeat -f to process multiple images
  -frame-ratio float
        Frame width as a fraction of the shorter image side (default 0.03)
  -no-frame
        Draw no border around the image
  -no-model
        Omit camera and lens data
  -o string
        Output JPEG path (single input only)
  -out-dir string
        Output directory (default ".")
  -preserve-exif
        Preserve source JPEG EXIF metadata in output
  -quality int
        JPEG quality (1-100, default 100)

# Example
$ go-exiframe -f /path/to/image.jpg
# Writes ./exiframe-image.jpg
$ go-exiframe -f /path/to/image.jpg -o /path/to/result.jpg -quality 90
$ go-exiframe -f ./first.jpg -f ./second.jpg -out-dir ./framed -preserve-exif
```

## 参考

- [go-exif/v3](https://pkg.go.dev/github.com/dsoprea/go-exif/v3)
- [CIPA DC-008-2024](https://www.cipa.jp/j/std/std-sec.html)
  - デジタルスチルカメラ用画像ファイルフォーマット規格 Exif 3.0
