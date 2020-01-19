package bundler

import (
	"esbuild/fs"
	"esbuild/logging"
	"esbuild/parser"
	"esbuild/resolver"
	"path"
	"testing"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func assertEmptyLog(t *testing.T, msgs []logging.Msg) {
	text := ""
	for _, msg := range msgs {
		text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
	}
	assertEqual(t, text, "")
}

type bundled struct {
	files         map[string]string
	entryPaths    []string
	expected      map[string]string
	parseOptions  parser.ParseOptions
	bundleOptions BundleOptions
}

func expectBundled(t *testing.T, args bundled) {
	t.Run("", func(t *testing.T) {
		resolver := resolver.NewResolver(fs.MockFS(args.files), []string{".jsx", ".js", ".json"})

		log, join := logging.NewDeferLog()
		args.parseOptions.IsBundling = true
		bundle := ScanBundle(log, resolver, args.entryPaths, args.parseOptions)
		assertEmptyLog(t, join())

		log, join = logging.NewDeferLog()
		args.bundleOptions.Bundle = true
		args.bundleOptions.omitLoaderForTests = true
		if args.bundleOptions.AbsOutputFile != "" {
			args.bundleOptions.AbsOutputDir = path.Dir(args.bundleOptions.AbsOutputFile)
		}
		results := bundle.Compile(log, args.bundleOptions)
		assertEmptyLog(t, join())

		assertEqual(t, len(results), len(args.expected))
		for _, result := range results {
			file := args.expected[result.JsAbsPath]
			path := "[" + result.JsAbsPath + "]\n"
			assertEqual(t, path+string(result.JsContents), path+file)
		}
	})
}

func TestSimpleES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				console.log(fn())
			`,
			"/foo.js": `
				export function fn() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  0() {
    // /foo.js
    function fn() {
      return 123;
    }

    // /entry.js
    console.log(fn());
  }
}, 0);
`,
		},
	})
}

func TestSimpleCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const fn = require('./foo')
				console.log(fn())
			`,
			"/foo.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  1(require, exports, module) {
    // /foo.js
    module.exports = function() {
      return 123;
    };
  },

  0(require) {
    // /entry.js
    const fn = require(1 /* ./foo */);
    console.log(fn());
  }
}, 0);
`,
		},
	})
}

func TestCommonJSFromES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const fn = require('./foo')
				console.log(fn())
			`,
			"/foo.js": `
				export function fn() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  1(require, exports) {
    // /foo.js
    require(exports, {
      fn: () => fn
    });
    function fn() {
      return 123;
    }
  },

  0(require) {
    // /entry.js
    const fn = require(1 /* ./foo */);
    console.log(fn());
  }
}, 0);
`,
		},
	})
}

func TestES6FromCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				console.log(fn())
			`,
			"/foo.js": `
				exports.fn = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  1(require, exports) {
    // /foo.js
    exports.fn = function() {
      return 123;
    };
  },

  0(require) {
    // /entry.js
    const foo = require(1 /* ./foo */);
    console.log(foo.fn());
  }
}, 0);
`,
		},
	})
}

func TestExportForms(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": `
				export const abc = undefined
			`,
			"/b.js": `
				export const xyz = null
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  2(require, exports) {
    // /a.js
    const abc = void 0;

    // /b.js
    var b = {};
    require(b, {
      xyz: () => xyz
    });
    const xyz = null;

    // /entry.js
    require(exports, {
      C: () => Class,
      Class: () => Class,
      Fn: () => Fn,
      abc: () => abc,
      b: () => b,
      c: () => c,
      default: () => default2,
      l: () => l,
      v: () => v
    });
    const default2 = 123;
    var v = 234;
    let l = 234;
    const c = 234;
    function Fn() {
    }
    class Class {
    }
  }
}, 2);
`,
		},
	})
}

func TestExportSelf(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  0(require, exports) {
    // /entry.js
    require(exports, {
      foo: () => foo
    });
    const foo = 123;
  }
}, 0);
`,
		},
	})
}

func TestExportSelfAsNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * as ns from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  0(require, exports) {
    // /entry.js
    require(exports, {
      foo: () => foo,
      ns: () => exports
    });
    const foo = 123;
  }
}, 0);
`,
		},
	})
}

func TestJSXImportsCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				module.exports = {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			JSX: parser.JSXOptions{
				Parse:    true,
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  0(require, exports, module) {
    // /custom-react.js
    module.exports = {};
  },

  1(require) {
    // /entry.js
    const custom_react = require(0 /* ./custom-react */);
    console.log(custom_react.elem("div", null), custom_react.elem(custom_react.frag, null, "fragment"));
  }
}, 1);
`,
		},
	})
}

func TestJSXImportsES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				export function elem() {}
				export function frag() {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			JSX: parser.JSXOptions{
				Parse:    true,
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `loader({
  1() {
    // /custom-react.js
    function elem() {
    }
    function frag() {
    }

    // /entry.js
    console.log(elem("div", null), elem(frag, null, "fragment"));
  }
}, 1);
`,
		},
	})
}

func TestNodeModules(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 1);
`,
		},
	})
}

func TestPackageJsonMain(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./custom-main.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/custom-main.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  0(require, exports, module) {
    // /Users/user/project/node_modules/demo-pkg/custom-main.js
    module.exports = function() {
      return 123;
    };
  },

  1(require) {
    // /Users/user/project/src/entry.js
    const demo_pkg = require(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 1);
`,
		},
	})
}

func TestTsconfigJsonBaseUrl(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `loader({
  1(require, exports, module) {
    // /Users/user/project/src/lib/util.js
    module.exports = function() {
      return 123;
    };
  },

  0(require) {
    // /Users/user/project/src/app/entry.js
    const util = require(1 /* lib/util */);
    console.log(util.default());
  }
}, 0);
`,
		},
	})
}