package vueglue

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
)

type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Type            string            `json:"type"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

type JSAppParams struct {
	JSHash        string `json:"hash"`
	ViteVersion   string `json:"vite_version"`
	ViteMajorVer  string `json:"vite_major_version"`
	PackageType   string `json:"package_type"`
	MajorVer      string `json:"major_version,omitempty"`
	EntryPoint    string `json:"entry_point"`
	HasTypeScript bool   `json:"has_ts"`
	IsVanilla     bool   `json:"is_vanilla,omitempty"`
	VueVersion    string `json:"vue_version,omitempty"`
	ReactVersion  string `json:"react_version,omitempty"`
	PreactVersion string `json:"preact_version,omitempty"`
	SvelteVersion string `json:"svelte_version,omitempty"`
	LitVersion    string `json:"lit_version,omitempty"`
}

func (vg *ViteConfig) parsePackageJSON() (*PackageJSON, error) {
	// If not set, try and find package.json
	path := ""
	if _, ok := vg.FS.(embed.FS); ok {
		path = vg.AssetsPath + "/"
	}
	buf, err := fs.ReadFile(vg.FS, path+"package.json")
	if err != nil {
		return nil, err
	}

	content := PackageJSON{}
	err = json.Unmarshal(buf, &content)
	if err != nil {
		return nil, err
	}

	return &content, nil
}

func analyzePackageJSON(pkgJSON *PackageJSON) *JSAppParams {
	semVer, err := regexp.Compile(`^\^((\d+)\.\d+\.\d+)$`)
	if err != nil {
		// remove once we verify the regexp
		panic(err)
	}

	// parse for a ver; return the full version,
	// and the major version. Empty strings if
	// the version does not fit our regexp.
	getSemVer := func(verStr string) (string, string) {
		matches := semVer.FindStringSubmatch(verStr)
		var major string
		var fullVers string
		if matches != nil {
			major = matches[1]
			fullVers = matches[2]
		}
		return major, fullVers
	}

	output := JSAppParams{}

	// Is this actually a Vite package.json?
	if viteVers, ok := pkgJSON.DevDependencies["vite"]; ok {
		major, full := getSemVer(viteVers)
		output.ViteMajorVer = major
		output.ViteVersion = full
	} else {
		// Can't do anything with this package.json
		return nil
	}

	// TS?
	_, ok := pkgJSON.DevDependencies["typescript"]
	if ok {
		output.HasTypeScript = true
	}

	supported := []string{
		"vue",
		"react",
		"preact",
		"svelte", // devdep!
		"lit",    // won't really support
	}

	var vers string
	for _, pkg := range supported {
		if pkg == "svelte" {
			// special cased because svelte does not put
			// any configuration into dependencies.
			if sVer, ok := pkgJSON.DevDependencies["svelte"]; ok {
				vers = sVer
				major, full := getSemVer(vers)
				output.PackageType = pkg
				output.MajorVer = major
				output.SvelteVersion = full

				entryPt := "src/main.js"
				if output.HasTypeScript {
					entryPt = "src/main.ts"
				}
				output.EntryPoint = entryPt
				break
			}
		} else {
			if vers, ok = pkgJSON.Dependencies[pkg]; ok {
				output.PackageType = pkg
				major, full := getSemVer(vers)
				output.MajorVer = major

				// handle by category
				entryPt := "src/main.js" // most common case
				switch pkg {
				case "vue":
					output.VueVersion = full
					if output.HasTypeScript {
						entryPt = "src/main.ts"
					}
				case "react":
					output.ReactVersion = full
					if output.HasTypeScript {
						entryPt = "src/main.tsx"
					} else {
						entryPt = "src/main.jsx"
					}
				case "preact":
					output.PreactVersion = full
					if output.HasTypeScript {
						entryPt = "src/main.tsx"
					} else {
						entryPt = "src/main.jsx"
					}
				case "lit":
					output.LitVersion = full
					// we do not set entryPt;
					// lit is just too weird.
					entryPt = ""
				}
				// We know as much as we can...
				output.EntryPoint = entryPt
				break
			}
		}

	}

	// If we do not have type, call it Vanilla
	if output.PackageType == "" {
		output.IsVanilla = true
		// Vite choses entry points anyway
		if output.HasTypeScript {
			output.EntryPoint = "src/main.ts"
		} else {
			output.EntryPoint = "src/main.js"
		}
	}

	// ViteVer

	// TS
	// script "build": "tsc && vite build",
	// OR: DD "typescript": "^4.6.4"

	// Vanilla
	// NO DD
	// entry like Vue

	// Vanilla TS
	// script + DD

	// VueVer
	//  deps "vue": "^3.2.37"
	// src/main.js, src/main.ts

	// ReactVer
	// deps "react": "^18.2.0"
	// src/main.jsx

	// React TS
	// src/main.tsx

	// Preact
	// dep "preact": "^10.9.0"
	// TS: script + DD
	// entry as react

	// SvelteVer
	// devdeps: "svelte": "^3.49.0",
	// entry: as Vue

	// SvelteTSVer
	// NO build step info
	// DD: "typescript": "^4.6.4"

	// Lit
	// Weird structure!
	// deps "lit": "^2.2.7"
	// TS script + DD
	// weird entry src/my-element.ts

	return &output
}

func (vg *ViteConfig) getViteVersion() (string, error) {
	// If it's set, use it.
	if vg.ViteVersion != "" {
		return vg.ViteVersion, nil
	}

	if vg.DevDefaults == nil {
		return "", errors.New("not Vite project")
	}
	vg.ViteVersion = vg.DevDefaults.ViteMajorVer
	return vg.DevDefaults.ViteMajorVer, nil

}

func (vg *ViteConfig) SetDevelopmentDefaults() error {
	// Make sure we can find package.json:
	if vg.AssetsPath == "" {
		vg.AssetsPath = "frontend"
	}
	pkgJSON, err := vg.parsePackageJSON()
	if err != nil {
		return err
	}
	defaults := analyzePackageJSON(pkgJSON)
	if defaults == nil {
		return errors.New("invalid configuration")
	}
	vg.DevDefaults = defaults
	version, err := vg.getViteVersion()
	if err != nil {
		vg.ViteVersion = DEFAULT_VITE_VERSION
		version = vg.ViteVersion
	}

	// Check for anything already set, and if not set,
	// use the defaults if they are not set.
	if vg.Platform == "" {
		vg.Platform = defaults.PackageType
	}

	if vg.EntryPoint == "" {
		vg.EntryPoint = defaults.EntryPoint
	}

	if vg.URLPrefix == "" {
		// Vite default
		vg.URLPrefix = "/src/"
	}

	if vg.DevServerPort == "" {
		if version == "2" {
			vg.DevServerPort = DEFAULT_PORT_V2
		} else {
			vg.DevServerPort = DEFAULT_PORT_V3
		}
	}

	if vg.DevServerDomain == "" {
		vg.DevServerDomain = "localhost"
	}

	return nil

}

func (vg *ViteConfig) buildDevServerBaseURL() string {
	protocol := "http"
	if vg.HTTPS {
		protocol = "https"
	}

	return fmt.Sprintf(
		"%s://%s:%s",
		protocol,
		vg.DevServerDomain,
		vg.DevServerPort,
	)

}
