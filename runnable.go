package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/Nixson/annotation"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

//go:embed tpl/*.goTpl
var tpls embed.FS

func main() {
	generate(Scan())
}

func Scan() map[string][]annotation.Element {

	fileSystem := os.DirFS(".")
	dirs := make([]string, 0)
	_ = fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() && path[0:1] != "." {
			dirs = append(dirs, path)
		}
		return nil
	})
	annotations := make([]annotation.Element, 0)
	for _, dir := range dirs {
		d, err := parser.ParseDir(token.NewFileSet(), dir, nil, parser.ParseComments)
		if err != nil {
			fmt.Println(err)
			continue
		}
		for k, f := range d {
			p := doc.New(f, k, 0)
			for _, tp := range p.Types {
				if tp.Doc != "" {
					annotationE := getAnnotation(tp.Name, tp.Doc, k)
					if annotationE.Type == "Controller" {
						annotationE.Children = make([]annotation.Element, 0)
						for _, method := range tp.Methods {
							if method.Doc != "" {
								annotationE.Children = append(annotationE.Children, getAnnotation(method.Name, method.Doc, k))
							}
						}
					}
					annotations = append(annotations, annotationE)
				}
			}
			for _, tp := range p.Funcs {
				if tp.Doc != "" {
					annotationE := getAnnotation(tp.Name, tp.Doc, k)
					if annotationE.Type == "KafkaListen" {
						annotations = append(annotations, annotationE)
					}
				}
			}
		}

	}
	if len(annotations) > 0 {
		annMap := make(map[string][]annotation.Element)
		annMap["controller"] = get("Controller", annotations)
		annMap["crud"] = get("CRUD", annotations)
		annMap["kafka"] = get("KafkaListen", annotations)
		annotationE, _ := os.Create("resources/annotation.json")
		writr := bufio.NewWriter(annotationE)
		b, err := json.Marshal(annMap)
		if err == nil {
			_, _ = writr.Write(b)
			_ = writr.Flush()
		}
		return annMap
	}
	return nil
}

func get(s string, annotations []annotation.Element) []annotation.Element {
	resp := make([]annotation.Element, 0)
	for _, annotationE := range annotations {
		if annotationE.Type == s {
			resp = append(resp, annotationE)
		}
	}
	return resp
}

func getAnnotation(name, in, url string) annotation.Element {
	ann := annotation.Element{
		StructName: name,
		Url:        url,
	}
	sep := strings.Split(in, "\n")
	for _, str := range sep {
		if strings.Contains(str, "@") {
			titleApp := strings.Split(str, "@")
			title := strings.TrimSpace(titleApp[1])
			if strings.Contains(title, "(") {
				strName := strings.Split(title, "(")
				ann.Type = strName[0]
				ann.Parameters = parseParams(strName[1])
			} else {
				ann.Type = title
			}
		}
	}
	return ann
}

func parseParams(s string) map[string]string {
	paramsMap := make(map[string]string)
	s = s[:len(s)-1]
	sep := strings.Split(s, ",")
	for _, substr := range sep {
		substr = strings.TrimSpace(substr)
		if !strings.Contains(substr, "=") {
			continue
		}
		keyVal := strings.Split(substr, "=")
		paramsMap[strings.TrimSpace(keyVal[0])] = strings.Trim(strings.TrimSpace(keyVal[1]), `"`)
	}
	return paramsMap
}

var isVar = regexp.MustCompile(`\$\{(.*?)\}`)

func generate(annotationMap map[string][]annotation.Element) {
	os.RemoveAll("vendor/gen")
	os.Mkdir("vendor/gen", os.ModePerm)
	modFile, err := os.ReadFile("go.mod")
	if err != nil {
		return
	}
	lines := strings.Split(string(modFile), "\n")
	moduleNames := strings.Split(lines[0], " ")
	moduleName := moduleNames[1]
	fmt.Println(moduleName)
	controller, okCtr := annotationMap["controller"]
	if okCtr {
		os.Mkdir("vendor/gen/controller", os.ModePerm)
		list := make([]string, 0)
		fileTpl, _ := tpls.ReadFile("tpl/controller.goTpl")
		fileTplStr := string(fileTpl)
		for _, element := range controller {
			mapCont := make(map[string]string)
			mapCont["path"] = moduleName + "/" + element.Url
			mapCont["name"] = element.StructName
			newFileContent := replace(fileTplStr, mapCont)
			f, _ := os.Create("vendor/gen/controller/" + strings.ToLower(element.StructName) + ".go")
			_, _ = f.WriteString(newFileContent)
			_ = f.Close()
			list = append(list, "InitController"+element.StructName+"()")
			fmt.Println("InitController" + element.StructName + "()")
		}
		fileMainTpl, _ := tpls.ReadFile("tpl/controllerMain.goTpl")
		mapCont := make(map[string]string)
		mapCont["initList"] = strings.Join(list, "\n\t")
		newFileContent := replace(string(fileMainTpl), mapCont)
		f, _ := os.Create("vendor/gen/controller/main.go")
		_, _ = f.WriteString(newFileContent)
		_ = f.Close()
	}
	listener, okListener := annotationMap["kafka"]
	if okListener {
		os.Mkdir("vendor/gen/listener", os.ModePerm)
		list := make([]string, 0)
		fileTpl, _ := tpls.ReadFile("tpl/kafka.goTpl")
		fileTplStr := string(fileTpl)
		for _, element := range listener {
			mapCont := make(map[string]string)
			mapCont["path"] = moduleName + "/" + element.Url
			mapCont["name"] = element.StructName
			mapCont["method"] = element.StructName
			mapCont["topic"] = element.Parameters["topic"]
			mapCont["group"] = element.Parameters["group"]
			newFileContent := replace(fileTplStr, mapCont)
			f, _ := os.Create("vendor/gen/listener/" + strings.ToLower(element.StructName) + ".go")
			_, _ = f.WriteString(newFileContent)
			_ = f.Close()
			list = append(list, element.StructName+"()")
			fmt.Println(element.StructName + "()")
		}
		fileMainTpl, _ := tpls.ReadFile("tpl/kafkaMain.goTpl")
		mapCont := make(map[string]string)
		mapCont["initList"] = strings.Join(list, "\n\t")
		newFileContent := replace(string(fileMainTpl), mapCont)
		f, _ := os.Create("vendor/gen/listener/main.go")
		_, _ = f.WriteString(newFileContent)
		_ = f.Close()
	}
	repository, okCrud := annotationMap["crud"]
	okMigration := false
	migrationList := make([]string, 0)
	if okCrud {
		os.Mkdir("vendor/gen/repository", os.ModePerm)
		fileTpl, _ := tpls.ReadFile("tpl/repository.goTpl")
		fileTplStr := string(fileTpl)
		for _, element := range repository {
			mapCont := make(map[string]string)
			mapCont["path"] = moduleName + "/" + element.Url
			mapCont["name"] = element.StructName
			mapCont["log"] = ""
			mapCont["table"] = strings.ToLower(element.StructName)
			tableName, ok := element.Parameters["table"]
			if ok {
				mapCont["table"] = tableName
			}
			mapCont["subname"] = strings.ToLower(element.StructName)
			newFileContent := replace(fileTplStr, mapCont)
			f, _ := os.Create("vendor/gen/repository/" + strings.ToLower(element.StructName) + ".go")
			_, _ = f.WriteString(newFileContent)
			_ = f.Close()
			fmt.Println(element.StructName)
			create, cok := element.Parameters["create"]
			if cok && create == "true" {
				okMigration = true
				migrationList = append(migrationList, "repository."+element.StructName+"Migrate()")
			}
		}
	}
	{
		envFile, _ := os.Create("vendor/gen/env.go")
		envTpl, _ := tpls.ReadFile("tpl/env.goTpl")
		mapCont := make(map[string]string)
		if okCtr {
			mapCont["controllerPath"] = "\"gen/controller\"\n\t"
			mapCont["controllerInit"] = `controller.InitServer()`
		} else {
			mapCont["controllerPath"] = ""
			mapCont["controllerInit"] = ""
		}
		if okListener {
			mapCont["listenerPath"] = "\"gen/listener\"\n\t"
			mapCont["listenerInit"] = `listener.InitKafka()`

		} else {
			mapCont["listenerPath"] = ""
			mapCont["listenerInit"] = ""
		}
		if okMigration {
			mapCont["migratePath"] = "\"gen/repository\"\n\t"
			mapCont["migrate"] = strings.Join(migrationList, "\n\t\t")
		} else {
			mapCont["migratePath"] = ""
			mapCont["migrate"] = ""
		}
		envFile.WriteString(replace(string(envTpl), mapCont))
		envFile.Close()
	}

}

func replace(content string, cont map[string]string) string {
	for name, data := range cont {
		content = strings.ReplaceAll(content, "${"+name+"}", data)
	}
	return content
}
