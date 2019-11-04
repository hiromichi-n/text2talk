package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
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
	spew.Dump(config)
	fmt.Println("\n config end")
}

var config viperConfig

type viperConfig struct {
	Image struct {
		Height   int
		Width    int
		TextTtf  string
		FontSize float64
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
		Rgb              [][]float64
		NewLineReplacers []string
	}
}

// TODO テキストとその秒数を表示する。
// TODO 字幕テキストも自動で作りたい。ついで画像も入れたい画像を合成する.
// 話すキャラ毎の色分けしたい
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
		outFileOrg := outDir + strconv.Itoa(k) + "_" + getHeadText(text)
		outFileWav := outFileOrg + ".wav"
		outFileImg := outFileOrg + ".png"
		cmdText, err := createVoice(text, outFileWav, voiceIndex)

		if err != nil {
			panic(err)
		}
		cmd := exec.Command("sh", "-c", string(cmdText))
		err = cmd.Start()
		if err != nil {
			panic(err)
		}
		cmd.Wait()
		err = createImage(text, outFileImg, voiceIndex)
		if err != nil {
			panic(err)
		}
		fmt.Println("file created :", outFileImg)
	}
}

func createImage(text, outPath string, voiceIndex int) error {
	dc := gg.NewContext(config.Image.Width, config.Image.Height)
	dc.SetRGBA(0, 0, 0, 0)
	dc.Clear()
	dc.SetRGB(config.Voice.Rgb[voiceIndex][0], config.Voice.Rgb[voiceIndex][1], config.Voice.Rgb[voiceIndex][2])

	if err := dc.LoadFontFace(config.Image.TextTtf, config.Image.FontSize); err != nil {
		panic(err)
	}
	dc.DrawStringAnchored(text, float64(config.Image.Width)/2, float64(config.Image.Height)/2, 0.5, 0.5)
	return dc.SavePNG(outPath)
}

func createVoice(text, outFilePaht string, voiceIndex int) (string, error) {
	cmdText, err := pipeline.Output(
		[]string{"echo", text},
		[]string{"/usr/local/bin/open_jtalk", "-x", config.Openjtalk.Dict, "-m", config.Voice.Hts[voiceIndex], "-ow", outFilePaht},
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
