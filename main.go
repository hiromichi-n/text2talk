package main

import (
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"github.com/mattn/go-pipeline"
	"github.com/spf13/viper"
)

// read a setting
func init() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("example")
	viper.SetConfigType("toml")
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Cannot read config file:%s", err))
	}
	err = viper.Unmarshal(&config)
	if err != nil {
		panic(fmt.Errorf("Cannot Unmarshal config file:%s", err))
	}
	fmt.Println("config info")
	fmt.Println("\n config end")
}

var config viperConfig

type viperConfig struct {
	Image struct {
		BaseHeight int
		BaseWidth  int
		TextTtf    string
	}
	Openjtalk struct {
		Dict string
	}
	Output struct {
		Directory        string
		FileNameHeadText int
	}
	ReadText struct {
		FilePath string
	}
	Voice struct {
		Hts              []string
		Rgb              [][]int
		NewLineReplacers []string
		Images           []string
	}
}

// 1行の文字数が多い時は二段にする
// 並列処理でいい感じにしたい...
func main() {
	text, err := readText(config.ReadText.FilePath)
	if err != nil {
		fmt.Println(err)
		return
	}
	outDir := config.Output.Directory
	cmd := exec.Command("mkdir", config.Output.Directory)
	cmd.Start()
	cmd.Wait()

	// 改行で区切る
	strList := strings.Split(text, "\n")
	for k, text := range strList {
		voiceIndex := 0
		// 誰の声かを特定する
		for index, replacer := range config.Voice.NewLineReplacers {
			if strings.HasPrefix(text, replacer) {
				voiceIndex = index
				text = strings.Replace(text, replacer, "", 1)
			}
		}

		if text == "" {
			continue
		}
		outFileBase := outDir + strconv.Itoa(k) + "_" + getHeadText(text)
		outWavFile := outFileBase + ".wav"
		outImgFile := outFileBase + ".png"
		cmdText, err := createVoice(text, outWavFile, voiceIndex)

		if err != nil {
			panic(err)
		}
		cmd := exec.Command("sh", "-c", string(cmdText))
		err = cmd.Start()
		if err != nil {
			panic(err)
		}
		cmd.Wait()
		err = createImage(text, outImgFile, voiceIndex)
		if err != nil {
			panic(err)
		}
		fmt.Println("file created :", outImgFile)
	}
}

// TODO 画像設定の読み込みは効率よくする。
func createImage(text, outPath string, voiceIndex int) error {
	dc := gg.NewContext(config.Image.BaseWidth, config.Image.BaseHeight)
	dc.SetRGBA(0, 0, 0, 0)
	dc.Clear()
	dc.SetRGB255(config.Voice.Rgb[voiceIndex][0], config.Voice.Rgb[voiceIndex][1], config.Voice.Rgb[voiceIndex][2])

	if err := dc.LoadFontFace(config.Image.TextTtf, getFontPoint(config.Image.BaseHeight)); err != nil {
		panic(err)
	}

	if len(config.Voice.Images) > voiceIndex {
		faceImage, err := readImage(config.Voice.Images[voiceIndex])
		if err != nil {
			return err
		}
		faceImage = imaging.Resize(faceImage, config.Image.BaseWidth/5, config.Image.BaseWidth/5, imaging.Lanczos)
		dc.DrawImageAnchored(faceImage, 0, config.Image.BaseHeight/12, float64(0.0), float64(0.0))
		fmt.Println("draw face finished")
	}
	err := drawText(dc, text, config.Image.BaseWidth, config.Image.BaseHeight)
	if err != nil {
		panic(err)
	}
	return dc.SavePNG(outPath)
}

// 文字はのフォントサイズを出力する
// 出力画像の1/3にして
func getFontPoint(height int) (points float64) {
	return getFontSizeFromImageHeight(height) * 96 / 72
}
func getFontSizeFromImageHeight(height int) float64 {
	return float64(height) / 6
}
func drawText(dc *gg.Context, text string, width, height int) (err error) {
	// 文字量が多い時は2行にする。
	textCount := utf8.RuneCountInString(text)
	// 文字の大きさ
	charSize := getFontSizeFromImageHeight(height)
	// 横に入りきる文字数を計算
	maxWidthChar := width * 4 / 5 / int(charSize)
	//最大横幅より文字数が少ない時はそのまま書き込む
	if textCount < maxWidthChar {
		dc.DrawString(text, float64(width)*0.22, charSize*3)
		return
	}
	// 2倍よりも多い時はエラーにする
	if textCount > maxWidthChar*2 {
		panic("too long 1 line text:" + text)
	}
	line1, line2 := get2LineText(text, maxWidthChar)
	// 2倍以下の時は2段にする
	dc.DrawString(line1, float64(width)*0.22, charSize*2)
	dc.DrawString(line2, float64(width)*0.22, charSize*4)
	return
}

const endPunctuation = "。"
const readingPunctuation = "、"

func get2LineText(text string, maxChar int) (line1, line2 string) {
	// 「。」がある時は「。」で区切る
	if strings.Contains(text, endPunctuation) {
		lines := strings.SplitN(text, endPunctuation, 2)
		return lines[0] + endPunctuation, lines[1]
	}
	return text, ""
}

func createVoice(text, outFilePath string, voiceIndex int) (string, error) {
	cmdText, err := pipeline.Output(
		[]string{"echo", text},
		[]string{"/usr/local/bin/open_jtalk", "-x", config.Openjtalk.Dict, "-m", config.Voice.Hts[voiceIndex], "-ow", outFilePath},
	)
	return string(cmdText), err
}

func getHeadText(str string) string {
	runes := []rune(str)
	max := config.Output.FileNameHeadText
	if len(runes) < config.Output.FileNameHeadText {
		max = len(runes)
	}
	return string(runes[0:max])
}

func readText(filePath string) (result string, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	// 一気に全部読み取り
	b, err := ioutil.ReadAll(file)
	return string(b), err
}

func readImage(filePath string) (result image.Image, err error) {
	result, err = imaging.Open(filePath)
	return
}
