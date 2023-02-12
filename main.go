package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"github.com/fatih/color"
)

const (
	IndexUrl = "https://ziglang.org/download/index.json"
)

func zigBinPath() string {
    return homeDirPath(".local", "bin", "zig")
}

func homeDirPath(p ... string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	return path.Join(append([]string{home}, p...)...)
}

func localDirPath(p ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	return path.Join(append([]string{home, ".zig-toolchain"}, p...)...)
}

func ensureDirectories() {
	var err error
	err = os.MkdirAll(localDirPath("tarballs"), os.ModePerm)
	err = os.MkdirAll(localDirPath("current"), os.ModePerm)
	if err != nil {
		panic(err)
	}
}

func getHostOs() string {
	os := runtime.GOOS
	switch os {
	case "windows":
		return os
	case "darwin":
		return "macos"
	case "linux":
		return os
	}

	panic("Invalid os!")
}

func getHostArch() string {
	arch := runtime.GOARCH
	switch arch {
	case "386":
		return "x86"
	case "amd64":
		return "x86-64"
	case "arm64":
		return "aarch64"
	}

	panic("Invalid arch!")
}

func localTarballPathFromUrl(url string) string {
	sp := strings.Split(url, "/")
	filename := sp[len(sp)-1]
	return localDirPath("tarballs", filename)
}

func extractedDirForVersion(v Version) string {
	fname := fmt.Sprintf("zig-%s-%s-%d.%d.%d", getHostOs(), getHostArch(), v.Major, v.Minor, v.Patch)
	if v.Dev {
		fname += fmt.Sprintf("-dev.%d+%s", v.Build, v.Commit)
	}
	return localDirPath("current", fname)
}

type Item struct {
	Version    Version
	Downloaded bool
	Current    bool
	Indexed    bool
	Master     bool
	LocalPath  string
	RemoteUrl  string
}

type Version struct {
	Major  int
	Minor  int
	Patch  int
	Dev    bool
	Build  int
	Commit string
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Dev {
		s += fmt.Sprintf("-dev-%d", v.Build)
	}
	return s
}

func (v Version) equal(other Version) bool {
	if v.Dev || other.Dev {
		if !(v.Dev && other.Dev) {
			return false
		}

		return v.Major == other.Major && v.Minor == other.Minor && v.Patch == other.Patch && v.Build == other.Build
	}

	return v.Major == other.Major && v.Minor == other.Minor && v.Patch == other.Patch
}

func (v Version) lessThan(other Version) bool {
	if v.Major == other.Major {
		if v.Minor == other.Minor {
			if v.Patch == other.Patch {
				if v.Dev && other.Dev {
					return v.Build < other.Build
				} else if v.Dev && !other.Dev {
					return false
				} else if !v.Dev && other.Dev {
					return true
				} else {
					return false
				}
			} else {
				return v.Patch < other.Patch
			}
		} else {
			return v.Minor < other.Minor
		}
	} else {
		return v.Major < other.Major
	}
}

func (v Version) moreThan(other Version) bool {
	return !v.lessThan(other)
}

// Given a version string in the form 0.10.1, or 0.11.2-dev-1234+a3f634,
// return the corresponding Version object.
func ParseVersion(v string) (*Version, error) {
	result := &Version{}

	sp := strings.Split(v, "-")
	sp2 := strings.Split(sp[0], ".")

	if len(sp2) != 3 {
		return nil, errors.New(fmt.Sprintf("Failed to parse version: %s", v))
	}

	major, err := strconv.ParseInt(sp2[0], 10, 32)
	minor, err := strconv.ParseInt(sp2[1], 10, 32)
	patch, err := strconv.ParseInt(sp2[2], 10, 32)

	if err != nil {
		return nil, err
	}

	result.Major = int(major)
	result.Minor = int(minor)
	result.Patch = int(patch)

	if len(sp) > 1 {
		result.Dev = true
		sp2 = strings.Split(strings.Split(sp[1], ".")[1], "+")
		build, err := strconv.ParseInt(sp2[0], 10, 32)
		if err != nil {
			return nil, err
		}
		result.Build = int(build)
		result.Commit = sp2[1]
	}

	return result, nil
}

type AppState struct {
	Items []Item
}

func (app *AppState) GetCurrentActiveItem() (*Item, bool) {
	for _, item := range app.Items {
		if item.Current {
			return &item, true
		}
	}

	return nil, false
}

func (app *AppState) GetItemByVersion(v Version) (*Item, bool) {
	for i := 0; i < len(app.Items); i++ {
		var item = &app.Items[i]
		if item.Version.equal(v) {
			return item, true
		}
	}

	return nil, false
}

func NewAppState() *AppState {
	return &AppState{Items: []Item{}}
}

type ZigIndex struct {
	Entries map[string]ZigIndexEntry
}

type ZigIndexEntry struct {
	Version           string             `json:"version"`
	Date              string             `json:"date"`
	Docs              string             `json:"docs"`
	StdDocs           string             `json:"stdDocs"`
	Src               *ZigIndexFileEntry `json:"src"`
	Bootstrap         *ZigIndexFileEntry `json:"bootstrap"`
	X86_64_macos      *ZigIndexFileEntry `json:"x86_64-macos"`
	Aarch64_macos     *ZigIndexFileEntry `json:"aarch64-macos"`
	X86_64_linux      *ZigIndexFileEntry `json:"x86_64-linux"`
	Aarch64_linux     *ZigIndexFileEntry `json:"aarch64-linux"`
	Riscv64_linux     *ZigIndexFileEntry `json:"riscv64-linux"`
	Powerpc64le_linux *ZigIndexFileEntry `json:"powerpc64le-linux"`
	Powerpc_linux     *ZigIndexFileEntry `json:"powerpc-linux"`
	X86_linux         *ZigIndexFileEntry `json:"x86-linux"`
	X86_64_windows    *ZigIndexFileEntry `json:"x86_64-windows"`
	Aarch64_windows   *ZigIndexFileEntry `json:"aarch64-windows"`
	X86_windows       *ZigIndexFileEntry `json:"x86-windows"`
}

func (z *ZigIndexEntry) GetFileEntryForHost() *ZigIndexFileEntry {
	os := getHostOs()
	arch := getHostArch()

	switch os {
	case "macos":
		switch arch {
		case "aarch64":
			return z.Aarch64_macos
		case "x86-64":
			return z.X86_64_macos
		}

	case "linux":
		switch arch {
		case "aarch64":
			return z.Aarch64_linux
		case "x86-64":
			return z.X86_64_linux
		case "x86":
			return z.X86_linux
		}

	case "windows":
		switch arch {
		case "aarch64":
			return z.Aarch64_windows
		case "x86-64":
			return z.X86_64_windows
		case "x86":
			return z.X86_windows
		}
	}

	panic("invalid os/arch!")
}

type ZigIndexFileEntry struct {
	Tarball string
	Shasum  string
	Size    string
}

func NewZigIndex() *ZigIndex {
	return &ZigIndex{
		Entries: make(map[string]ZigIndexEntry, 0),
	}
}

func FetchIndex() (*ZigIndex, error) {
	result := NewZigIndex()

	// Download the JSON file
	resp, err := http.Get(IndexUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the body of the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// var f map[string]ZigIndexEntry
	err = json.Unmarshal(body, &result.Entries)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (app *AppState) commandListRemote() {
    green := color.New(color.FgGreen).SprintFunc()
    blue := color.New(color.FgBlue).SprintFunc()
    red := color.New(color.FgRed).SprintFunc()
	fmt.Printf("List of indexed zig versions (%s %s):  \n\n", green("[active]"), blue("[downloaded]"))
	for _, item := range app.Items {
		if item.Indexed {
            if item.Current {
                fmt.Printf("%s %s", green("==>"), green(item.Version.String()))
            } else if item.Downloaded {
                fmt.Printf("%s %s", blue("==>"), blue(item.Version.String()))
            } else {
                fmt.Printf("==> %s", item.Version.String())
            }

            if item.Master {
                fmt.Printf(" %s ", red("[master]"))
            }

			fmt.Printf("\n")
		}
	}
}

func (app *AppState) commandListLocal() {
    green := color.New(color.FgGreen).SprintFunc()
    red := color.New(color.FgRed).SprintFunc()
    fmt.Printf("List of downloaded zig versions (%s): \n\n", green("[active]"))
	for _, item := range app.Items {
		if item.Downloaded {
			// fmt.Printf("  -%s", item.Version.String())
			// if item.Current {
			// 	fmt.Printf(" [current]")
			// }

            if item.Current {
                fmt.Printf("%s %s", green("==>"), green(item.Version.String()))
            } else {
                fmt.Printf("==> %s", item.Version.String())
            }

            if item.Master {
                fmt.Printf(" %s ", red("[master]"))
            }

			fmt.Printf("\n")
		}
	}
}

func (app *AppState) downloadTarball(item Item) error {
	fmt.Printf("Downlading tarball %s...", item.RemoteUrl)
	res, err := http.Get(item.RemoteUrl)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	file, err := os.Create(item.LocalPath)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err != nil {
		return err
	}

	fmt.Printf("Done!\n")

	return nil
}

func (app *AppState) commandDownloadMaster() {
	for i := 0; i < len(app.Items); i++ {
		item := &app.Items[i]
		if item.Master {
			app.commandDownloadItem(item)
			return
		}
	}

	panic("Master version not found!")
}

func (app *AppState) commandDownloadVersion(v Version) {
	if item, ok := app.GetItemByVersion(v); ok {
		app.commandDownloadItem(item)
	} else {
		fmt.Printf("Invalid version!")
		os.Exit(1)
	}
}

func (app *AppState) commandDownloadItem(item *Item) {
	if item.Downloaded {
		fmt.Print("Tarball already downloaded!\n")
		return
	}

	if !item.Indexed {
		fmt.Printf("Item not indexed!")
		os.Exit(1)
	}

	err := app.downloadTarball(*item)
	if err != nil {
		panic(err)
	}

	item.Downloaded = true
}

func (app *AppState) commandActivateMaster() {
	for i := 0; i < len(app.Items); i++ {
		item := &app.Items[i]
		if item.Master {
			app.commandActivateItem(item)
			return
		}
	}

	fmt.Printf("Version not found!\n")
	os.Exit(1)
}

func (app *AppState) commandActivateVersion(v Version) {
	item, ok := app.GetItemByVersion(v)
	if !ok {
		fmt.Printf("Version not found!\n")
		os.Exit(1)
	}
    app.commandActivateItem(item)
}

func (app *AppState) commandActivateItem(item *Item) {
	if item.Current {
		fmt.Printf("Version is already active!")
		os.Exit(0)
	}

	if !item.Downloaded {
		app.commandDownloadItem(item)
	}

    os.RemoveAll(localDirPath("current"))
    ensureDirectories()


    fmt.Printf("Extracting...")
	cmd := exec.Command("tar", "-xf", item.LocalPath)
	cmd.Dir = localDirPath("current")
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(string(out))
	}
    fmt.Printf("Done!\n")

    // link
    fmt.Printf("Creating symlink...")
    _, err =  os.Lstat(zigBinPath())
    if err == nil {
        err = os.Remove(zigBinPath())
        if err != nil {
            panic(err)
        }
    }
    err = os.Symlink(path.Join(extractedDirForVersion(item.Version), "zig"), zigBinPath())
    if err != nil {
        panic(err)
    }
    fmt.Printf("Done!\n")
}

const (
	CommandDownload = iota
	CommandList
	CommandShow
	CommandActivate
	CommandNone
)

func printUsageAndExit() {
	fmt.Printf("USAGE: zig-toolchain [COMMAND]\n\n")
	fmt.Printf("COMMANDS:")
	fmt.Printf("\n    download\t\t Download a zig version.")
	fmt.Printf("\n    list\t\t List remote versions.")
	fmt.Printf("\n    show\t\t List local versions.")
	fmt.Printf("\n    activate\t\t Activeate a given zig version.")
	fmt.Printf("\n\n")
	os.Exit(0)
}

func (app *AppState) run() {

	if len(os.Args) < 2 {
        printUsageAndExit()
	}

	command := CommandNone

	switch os.Args[1] {
	case "download":
		command = CommandDownload
	case "list":
		command = CommandList
	case "show":
		command = CommandShow
	case "activate":
		command = CommandActivate
	default:
		printUsageAndExit()
	}

	// Make sure local directories exist
	ensureDirectories()

	// Load remote data
	{
		var err error
		// Fetch remote index
		index, err := FetchIndex()
		if err != nil {
			panic(err)
		}

		// Parse remote index items
		for k, v := range index.Entries {
			fileEntry := v.GetFileEntryForHost()
			if fileEntry == nil {
				continue
			}
			item := Item{}

			versionString := v.Version
			if versionString == "" {
				versionString = k
			} else {
				item.Master = true
			}

			version, err := ParseVersion(versionString)
			if err != nil {
				panic(err)
			}

			item.Version = *version
			item.Indexed = true
			item.RemoteUrl = fileEntry.Tarball
			item.LocalPath = localTarballPathFromUrl(item.RemoteUrl)

			app.Items = append(app.Items, item)
		}
	}

	// Scan local tarballs
	{
		dir, err := os.ReadDir(localDirPath("tarballs"))
		if err != nil {
			panic(err)
		}

		for _, entry := range dir {
			name := entry.Name()
			if path.Ext(name) == ".xz" {
				sp := strings.Split(name, ".")
				name = strings.Join(sp[0:len(sp)-2], ".")
				sp = strings.Split(name, "-")
				// ostag := sp[1]
				// archtag := sp[2]
				versionTag := strings.Join(sp[3:], "-")

				version, err := ParseVersion(versionTag)
				if err != nil {
					panic(err)
				}

				// fmt.Printf("%s, %s, %+v\n", ostag, archtag, *version)

				if item, ok := app.GetItemByVersion(*version); ok {
					item.Downloaded = true
					item.LocalPath = localDirPath("tarballs", entry.Name())
				} else {
					item := Item{}
					item.Downloaded = true
					item.Indexed = false
					item.LocalPath = localDirPath("tarballs", entry.Name())
					item.Version = *version
					app.Items = append(app.Items, item)
				}
			}
		}
	}

	// look for current zig
	{
		dir, err := os.ReadDir(localDirPath("current"))
		if err != nil {
			panic(err)
		}

		if len(dir) > 0 {
			for _, e := range dir {
				if strings.HasPrefix(e.Name(), "zig") && e.IsDir() {
					name := e.Name()
					sp := strings.Split(name, "-")
					// ostag := sp[1]
					// archtag := sp[2]
					versionTag := strings.Join(sp[3:], "-")

					version, err := ParseVersion(versionTag)
					if err != nil {
						panic(err)
					}

					if item, ok := app.GetItemByVersion(*version); ok {
						item.Current = true
					} else {
						panic("current version is not downloaded!")
					}
					break
				}
			}
		}
	}

	// Sort items
	{
		sort.Slice(app.Items, func(i, j int) bool {
			return app.Items[i].Version.moreThan(app.Items[j].Version)
		})
	}

	switch command {
	case CommandList:
		app.commandListRemote()
	case CommandShow:
		app.commandListLocal()
	case CommandDownload:

		if len(os.Args) < 3 {
			fmt.Printf("USAGE: zig-toolchain download [VERSION]\n\n")
			os.Exit(0)
		}

		if os.Args[2] == "master" {
			app.commandDownloadMaster()
		} else {
			var v *Version
			var err error
			if v, err = ParseVersion(os.Args[2]); err != nil {
				fmt.Printf("Invalid version!\n")
				os.Exit(1)
			}
			app.commandDownloadVersion(*v)
		}

	case CommandActivate:

		if len(os.Args) < 3 {
			fmt.Printf("USAGE: zig-toolchain activate [VERSION]\n\n")
			os.Exit(0)
		}

		if os.Args[2] == "master" {
			app.commandActivateMaster()
		} else {
			var v *Version
			var err error
			if v, err = ParseVersion(os.Args[2]); err != nil {
				fmt.Printf("Invalid version!\n")
				os.Exit(1)
			}
			app.commandActivateVersion(*v)
		}
	}

	// app.commandDownloadVersion(0, 9, 0)
	// app.commandDownloadMaster()
	// app.commandListRemote()
	// app.commandListLocal()

}

func main() {
	app := NewAppState()
	app.run()
}

