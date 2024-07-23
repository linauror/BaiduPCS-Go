package pcscommand

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/pcstime"
)

type (
	// LsOptions 列目录可选项
	LsOptions struct {
		Total bool
	}

	// SearchOptions 搜索可选项
	SearchOptions struct {
		Total   bool
		Recurse bool
	}
)

const (
	opLs int = iota
	opSearch
)

// RunLs 执行列目录
func RunLs(pcspath string, lsOptions *LsOptions, orderOptions *baidupcs.OrderOptions) {
	err := matchPathByShellPatternOnce(&pcspath)
	if err != nil {
		fmt.Println(err)
		return
	}

	files, err := GetBaiduPCS().FilesDirectoriesList(pcspath, orderOptions)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("\n当前目录: %s\n----\n", pcspath)

	if lsOptions == nil {
		lsOptions = &LsOptions{}
	}

	renderTable(opLs, lsOptions.Total, pcspath, files)
	return
}

// RunSearch 执行搜索
func RunSearch(targetPath, keyword string, opt *SearchOptions) {
	err := matchPathByShellPatternOnce(&targetPath)
	if err != nil {
		fmt.Println(err)
		return
	}

	if opt == nil {
		opt = &SearchOptions{}
	}

	files, err := GetBaiduPCS().Search(targetPath, keyword, opt.Recurse)
	if err != nil {
		fmt.Println(err)
		return
	}

	renderTable(opSearch, opt.Total, targetPath, files)
	return
}

var _FN, _DN, _size int64 // 总文件数，目录数，容量大小

// RunCreateFileIndex 执行生成文件索引
func RunCreateFileIndex(pcspath string) {
	err := matchPathByShellPatternOnce(&pcspath)
	if err != nil {
		fmt.Println(err)
		return
	}

	// 1. 生成目录文件
	currentPath := strings.Split(pcspath, "/")[len(strings.Split(pcspath, "/"))-1] // 最后一层目录
	indexFile := currentPath + "_目录.txt"
	fmt.Println("1, 开始扫描生成索引文件，" + indexFile + "\n")
	_, err = os.Stat(indexFile) // 检测索引文件是否存在
	if err != nil {
		indexFile, err := os.OpenFile(indexFile, os.O_CREATE|os.O_WRONLY, 0666) // 创建索引文件
		if err != nil {
			fmt.Println(err)
			return
		}
		defer indexFile.Close()

		// 写入第一行占位行
		_, err = indexFile.WriteString(strings.Repeat(" ", 90) + "\n")
		if err != nil {
			return
		}

		err = renderIndexFile(indexFile, pcspath, currentPath, 1)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		// 写入总计数据
		indexFile.Seek(0, 0)
		_, err = indexFile.WriteString(fmt.Sprintf("文件夹数量：%d, 文件总数：%d，大小：%s", _DN, _FN, converter.ConvertFileSize(_size, 2)))
		if err != nil {
			return
		}

		fmt.Println("索引文件生成成功")
	} else {
		fmt.Println("索引文件已存在，继续")
	}

	// 2. 上传索引文件
	fmt.Println("\n2, 上传索引文件")
	RunUpload([]string{indexFile}, pcspath, &UploadOptions{Policy: "overwrite"})

	// 3. 分享索引文件，输出分享链接
	fmt.Println("\n3, 生成索引文件分享链接\n")
	shared, err := GetBaiduPCS().ShareSet([]string{pcspath}, &baidupcs.ShareOption{IsCombined: true})
	if err != nil {
		fmt.Printf("%s失败: %s\n", baidupcs.OperationShareSet, err)
		return
	}
	fmt.Printf("shareID: %d, 链接: %s?pwd=%s\n\n", shared.ShareID, shared.Link, shared.Pwd)

	return
}

func renderIndexFile(indexFile *os.File, pcspath, currentPath string, level int) (err error) {
	_, err = indexFile.WriteString(strings.Repeat("|  ", level-1) + "|—" + currentPath + "\n")
	if err != nil {
		return
	}

	fmt.Println("list " + pcspath)
	files, err := GetBaiduPCS().FilesDirectoriesList(pcspath, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	if len(files) > 0 {
		fileN, directoryN := files.Count()
		_FN = _FN + fileN
		_DN = _DN + directoryN
		_size = _size + files.TotalSize()

		for _, file := range files {
			if file.Isdir { // 是目录的时候继续处理
				time.Sleep(time.Millisecond * 500)
				if err = renderIndexFile(indexFile, pcspath+"/"+file.Filename, file.Filename, level+1); err != nil {
					fmt.Println(err.Error())
					return
				}
				continue
			}

			_, err = indexFile.WriteString(strings.Repeat("|  ", level+1) + file.Filename + " (" + converter.ConvertFileSize(file.Size, 2) + ")" + "\n")
			if err != nil {
				fmt.Println(err.Error())
				return
			}
		}
	}

	return nil
}

func renderTable(op int, isTotal bool, path string, files baidupcs.FileDirectoryList) {
	tb := pcstable.NewTable(os.Stdout)
	var (
		fN, dN   int64
		showPath string
	)

	switch op {
	case opLs:
		showPath = "文件(目录)"
	case opSearch:
		showPath = "路径"
	}

	if isTotal {
		tb.SetHeader([]string{"#", "fs_id", "app_id", "文件大小", "创建日期", "修改日期", "md5(截图请打码)", showPath})
		tb.SetColumnAlignment([]int{tablewriter.ALIGN_DEFAULT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT})
		for k, file := range files {
			if file.Isdir {
				tb.Append([]string{strconv.Itoa(k), strconv.FormatInt(file.FsID, 10), strconv.FormatInt(file.AppID, 10), "-", pcstime.FormatTime(file.Ctime), pcstime.FormatTime(file.Mtime), file.MD5, file.Filename + baidupcs.PathSeparator})
				continue
			}

			var md5 string
			if len(file.BlockList) > 1 {
				md5 = "(可能不正确)" + file.MD5
			} else {
				md5 = file.MD5
			}

			switch op {
			case opLs:
				tb.Append([]string{strconv.Itoa(k), strconv.FormatInt(file.FsID, 10), strconv.FormatInt(file.AppID, 10), converter.ConvertFileSize(file.Size, 2), pcstime.FormatTime(file.Ctime), pcstime.FormatTime(file.Mtime), md5, file.Filename})
			case opSearch:
				tb.Append([]string{strconv.Itoa(k), strconv.FormatInt(file.FsID, 10), strconv.FormatInt(file.AppID, 10), converter.ConvertFileSize(file.Size, 2), pcstime.FormatTime(file.Ctime), pcstime.FormatTime(file.Mtime), md5, file.Path})
			}
		}
		fN, dN = files.Count()
		tb.Append([]string{"", "", "总: " + converter.ConvertFileSize(files.TotalSize(), 2), "", "", "", fmt.Sprintf("文件总数: %d, 目录总数: %d", fN, dN)})
	} else {
		tb.SetHeader([]string{"#", "文件大小", "修改日期", showPath})
		tb.SetColumnAlignment([]int{tablewriter.ALIGN_DEFAULT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT})
		for k, file := range files {
			if file.Isdir {
				tb.Append([]string{strconv.Itoa(k), "-", pcstime.FormatTime(file.Mtime), file.Filename + baidupcs.PathSeparator})
				continue
			}

			switch op {
			case opLs:
				tb.Append([]string{strconv.Itoa(k), converter.ConvertFileSize(file.Size, 2), pcstime.FormatTime(file.Mtime), file.Filename})
			case opSearch:
				tb.Append([]string{strconv.Itoa(k), converter.ConvertFileSize(file.Size, 2), pcstime.FormatTime(file.Mtime), file.Path})
			}
		}
		fN, dN = files.Count()
		tb.Append([]string{"", "总: " + converter.ConvertFileSize(files.TotalSize(), 2), "", fmt.Sprintf("文件总数: %d, 目录总数: %d", fN, dN)})
	}

	tb.Render()

	if fN+dN >= 50 {
		fmt.Printf("\n当前目录: %s\n", path)
	}

	fmt.Printf("----\n")
}
