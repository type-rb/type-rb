// ==UserScript==
// @name         TypeRB syntax highlighting for GitHub
// @namespace    https://type-rb.github.io/
// @version      0.2.2
// @description  Highlight TypeRB files, pull request diffs, and Markdown code blocks on GitHub.
// @author       TypeRB contributors
// @license      MIT
// @homepageURL  https://github.com/type-rb/type-rb/tree/main/editors/github
// @supportURL   https://github.com/type-rb/type-rb/issues
// @updateURL    https://raw.githubusercontent.com/type-rb/type-rb/main/editors/github/typerb-github.user.js
// @downloadURL  https://raw.githubusercontent.com/type-rb/type-rb/main/editors/github/typerb-github.user.js
// @match        https://github.com/*
// @run-at       document-idle
// @sandbox      DOM
// @grant        GM_addStyle
// @noframes
// ==/UserScript==
/*
Bundled license notices

TypeRB
======
MIT License

Copyright (c) 2026 TypeRB contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


@shikijs/engine-javascript 4.4.3
================================
MIT License

Copyright (c) 2021 Pine Wu
Copyright (c) 2023 Anthony Fu <https://github.com/antfu>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


@shikijs/vscode-textmate 10.0.2
===============================
The MIT License (MIT)

Copyright (c) Microsoft Corporation

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


oniguruma-parser 0.12.2
=======================
MIT License

Copyright (c) 2025-2026 Steven Levithan

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


oniguruma-to-es 4.3.6
=====================
MIT License

Copyright (c) 2024-2026 Steven Levithan

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


regex 6.1.0
===========
MIT License

Copyright (c) 2025 Steven Levithan

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


regex-recursion 6.0.2
=====================
MIT License

Copyright (c) 2025 Steven Levithan

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


regex-utilities 2.3.0
=====================
MIT License

Copyright (c) 2024 Steven Levithan

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/
(()=>{var gt=`{
  "$schema": "https://raw.githubusercontent.com/martinring/tmlanguage/master/tmlanguage.json",
  "name": "TypeRB",
  "scopeName": "source.trb",
  "fileTypes": [
    "trb"
  ],
  "patterns": [
    {
      "include": "#comments"
    },
    {
      "include": "#heredocs"
    },
    {
      "include": "#strings"
    },
    {
      "include": "#imports"
    },
    {
      "include": "#declarations"
    },
    {
      "include": "#qualified-members"
    },
    {
      "include": "#keywords"
    },
    {
      "include": "#numbers"
    },
    {
      "include": "#symbols"
    },
    {
      "include": "#variables"
    },
    {
      "include": "#types"
    },
    {
      "include": "#constants"
    },
    {
      "include": "#callables"
    },
    {
      "include": "#operators"
    },
    {
      "include": "#punctuation"
    }
  ],
  "repository": {
    "comments": {
      "patterns": [
        {
          "name": "comment.line.number-sign.trb",
          "begin": "#",
          "beginCaptures": {
            "0": {
              "name": "punctuation.definition.comment.trb"
            }
          },
          "end": "$"
        }
      ]
    },
    "heredocs": {
      "patterns": [
        {
          "name": "string.unquoted.heredoc.trb",
          "begin": "(<<[-~]?)(['\\"\`]?)([\\\\p{L}_][\\\\p{L}\\\\p{N}_]*)(\\\\2)\\\\s*$",
          "beginCaptures": {
            "1": {
              "name": "punctuation.definition.string.begin.trb"
            },
            "2": {
              "name": "punctuation.definition.string.begin.trb"
            },
            "3": {
              "name": "entity.name.tag.heredoc.trb"
            },
            "4": {
              "name": "punctuation.definition.string.begin.trb"
            }
          },
          "end": "^\\\\s*\\\\3\\\\s*$",
          "endCaptures": {
            "0": {
              "name": "punctuation.definition.string.end.trb"
            }
          },
          "patterns": [
            {
              "include": "#escapes"
            },
            {
              "include": "#interpolations"
            }
          ]
        }
      ]
    },
    "strings": {
      "patterns": [
        {
          "name": "string.quoted.double.trb",
          "begin": "\\"",
          "beginCaptures": {
            "0": {
              "name": "punctuation.definition.string.begin.trb"
            }
          },
          "end": "\\"",
          "endCaptures": {
            "0": {
              "name": "punctuation.definition.string.end.trb"
            }
          },
          "patterns": [
            {
              "include": "#escapes"
            },
            {
              "include": "#interpolations"
            }
          ]
        },
        {
          "name": "string.quoted.single.trb",
          "begin": "'",
          "beginCaptures": {
            "0": {
              "name": "punctuation.definition.string.begin.trb"
            }
          },
          "end": "'",
          "endCaptures": {
            "0": {
              "name": "punctuation.definition.string.end.trb"
            }
          },
          "patterns": [
            {
              "include": "#escapes"
            }
          ]
        },
        {
          "name": "string.interpolated.command.trb",
          "begin": "\`",
          "beginCaptures": {
            "0": {
              "name": "punctuation.definition.string.begin.trb"
            }
          },
          "end": "\`",
          "endCaptures": {
            "0": {
              "name": "punctuation.definition.string.end.trb"
            }
          },
          "patterns": [
            {
              "include": "#escapes"
            },
            {
              "include": "#interpolations"
            }
          ]
        }
      ]
    },
    "escapes": {
      "patterns": [
        {
          "name": "constant.character.escape.trb",
          "match": "\\\\\\\\."
        }
      ]
    },
    "interpolations": {
      "patterns": [
        {
          "name": "meta.interpolation.trb",
          "begin": "#\\\\{",
          "beginCaptures": {
            "0": {
              "name": "punctuation.section.interpolation.begin.trb"
            }
          },
          "end": "\\\\}",
          "endCaptures": {
            "0": {
              "name": "punctuation.section.interpolation.end.trb"
            }
          },
          "patterns": [
            {
              "include": "#balanced-braces"
            },
            {
              "include": "$self"
            }
          ]
        }
      ]
    },
    "balanced-braces": {
      "patterns": [
        {
          "name": "meta.braces.trb",
          "begin": "\\\\{",
          "beginCaptures": {
            "0": {
              "name": "punctuation.section.braces.begin.trb"
            }
          },
          "end": "\\\\}",
          "endCaptures": {
            "0": {
              "name": "punctuation.section.braces.end.trb"
            }
          },
          "patterns": [
            {
              "include": "$self"
            }
          ]
        }
      ]
    },
    "imports": {
      "patterns": [
        {
          "match": "\\\\b(import)\\\\s+(?!\\\\{)([\\\\p{L}_][\\\\p{L}\\\\p{N}_-]*(?:/[\\\\p{L}_][\\\\p{L}\\\\p{N}_-]*)*)",
          "captures": {
            "1": {
              "name": "keyword.control.import.trb"
            },
            "2": {
              "name": "string.unquoted.import-path.trb"
            }
          }
        },
        {
          "match": "\\\\b(from)\\\\s+([\\\\p{L}_][\\\\p{L}\\\\p{N}_-]*(?:/[\\\\p{L}_][\\\\p{L}\\\\p{N}_-]*)*)",
          "captures": {
            "1": {
              "name": "keyword.control.import.trb"
            },
            "2": {
              "name": "string.unquoted.import-path.trb"
            }
          }
        }
      ]
    },
    "declarations": {
      "patterns": [
        {
          "match": "\\\\b(class|record|enum|module|interface)\\\\s+([\\\\p{L}_][\\\\p{L}\\\\p{N}_]*)",
          "captures": {
            "1": {
              "name": "storage.type.trb"
            },
            "2": {
              "name": "entity.name.type.trb"
            }
          }
        },
        {
          "match": "\\\\b(def)\\\\s+(?:(self)(\\\\.))?([\\\\p{L}_][\\\\p{L}\\\\p{N}_]*(?:[?!])?|\\\\[\\\\])",
          "captures": {
            "1": {
              "name": "storage.type.function.trb"
            },
            "2": {
              "name": "variable.language.self.trb"
            },
            "3": {
              "name": "punctuation.accessor.dot.trb"
            },
            "4": {
              "name": "entity.name.function.trb"
            }
          }
        }
      ]
    },
    "qualified-members": {
      "patterns": [
        {
          "match": "\\\\b([A-Z][A-Za-z0-9_]*)(::)([A-Z][A-Za-z0-9_]*)\\\\b",
          "captures": {
            "1": {
              "name": "entity.name.type.trb"
            },
            "2": {
              "name": "punctuation.accessor.namespace.trb"
            },
            "3": {
              "name": "constant.other.enum-member.trb"
            }
          }
        }
      ]
    },
    "keywords": {
      "patterns": [
        {
          "name": "storage.modifier.trb",
          "match": "\\\\b(?:mut|readonly|implements)\\\\b"
        },
        {
          "name": "storage.type.trb",
          "match": "\\\\b(?:class|record|enum|module|interface|def|fn)\\\\b"
        },
        {
          "name": "keyword.control.import.trb",
          "match": "\\\\b(?:import|from|as)\\\\b"
        },
        {
          "name": "keyword.control.trb",
          "match": "\\\\b(?:if|elsif|else|then|case|when|while|do|try|catch|return|break|next|end)(?![?!_=])\\\\b"
        },
        {
          "name": "keyword.operator.logical.trb",
          "match": "\\\\b(?:and|or|not)\\\\b"
        },
        {
          "name": "constant.language.boolean.trb",
          "match": "\\\\b(?:true|false)\\\\b"
        },
        {
          "name": "constant.language.nil.trb",
          "match": "\\\\bnil\\\\b"
        },
        {
          "name": "variable.language.self.trb",
          "match": "\\\\bself\\\\b"
        }
      ]
    },
    "numbers": {
      "patterns": [
        {
          "name": "constant.numeric.trb",
          "match": "\\\\b[0-9](?:_?[0-9])*(?:\\\\.[0-9](?:_?[0-9])*)?\\\\b"
        }
      ]
    },
    "symbols": {
      "patterns": [
        {
          "name": "constant.other.symbol.trb",
          "match": "(?<!:):(?![:=])[\\\\p{L}_][\\\\p{L}\\\\p{N}_]*[?!]?"
        }
      ]
    },
    "variables": {
      "patterns": [
        {
          "name": "variable.other.instance.trb",
          "match": "@[\\\\p{L}_][\\\\p{L}\\\\p{N}_]*"
        }
      ]
    },
    "types": {
      "patterns": [
        {
          "name": "support.type.builtin.trb",
          "match": "\\\\b(?:Any|Array|Boolean|Bytes|Float|Hash|Integer|Iterable|Range|String|StringBuilder)\\\\b"
        },
        {
          "name": "entity.name.type.parameter.trb",
          "match": "\\\\b[A-Z]\\\\b"
        },
        {
          "name": "entity.name.type.trb",
          "match": "\\\\b(?=[A-Z][A-Za-z0-9_]*\\\\b)(?=[A-Za-z0-9_]*[a-z])[A-Z][A-Za-z0-9_]*\\\\b"
        }
      ]
    },
    "constants": {
      "patterns": [
        {
          "name": "constant.other.trb",
          "match": "\\\\b[A-Z][A-Z0-9_]*\\\\b"
        }
      ]
    },
    "callables": {
      "patterns": [
        {
          "name": "support.function.builtin.trb",
          "match": "\\\\bputs(?=\\\\s*\\\\()"
        },
        {
          "match": "(\\\\.|&\\\\.)([\\\\p{L}_][\\\\p{L}\\\\p{N}_]*[?!]?)(?=\\\\s*(?:\\\\(|\\\\b))",
          "captures": {
            "1": {
              "name": "punctuation.accessor.dot.trb"
            },
            "2": {
              "name": "entity.name.function.method.trb"
            }
          }
        },
        {
          "name": "entity.name.function.call.trb",
          "match": "\\\\b[\\\\p{L}_][\\\\p{L}\\\\p{N}_]*[?!]?(?=\\\\s*\\\\()"
        }
      ]
    },
    "operators": {
      "patterns": [
        {
          "name": "keyword.operator.trb",
          "match": "<=>|\\\\.\\\\.\\\\.|\\\\.\\\\.|\\\\*\\\\*=|&&=|\\\\|\\\\|=|&\\\\.|::|:=|->|==|!=|<=|>=|=>|&&|\\\\|\\\\||\\\\*\\\\*|<<|>>|[=+\\\\-*/%<>!&|^~?]"
        }
      ]
    },
    "punctuation": {
      "patterns": [
        {
          "name": "punctuation.section.parens.trb",
          "match": "[()]"
        },
        {
          "name": "punctuation.section.brackets.trb",
          "match": "[\\\\[\\\\]]"
        },
        {
          "name": "punctuation.section.braces.trb",
          "match": "[{}]"
        },
        {
          "name": "punctuation.separator.trb",
          "match": "[,;:]"
        }
      ]
    }
  }
}
`;var qn=/[\u200e\u200f\u202a-\u202e\u2066-\u2069]/g,Hn=/^[0-9a-f]{40}$/;function z(e){return typeof e=="string"&&e.toLowerCase().endsWith(".trb")}function Vn(e){let t=e.split("/").filter(Boolean);return t.length<2?null:{owner:t[0],name:t[1]}}function Zn(e){return z(e)?e.split("/").every(n=>n.length>0&&n!=="."&&n!==".."):!1}function Xn(e,t,n,r){if(!t||!Hn.test(n)||!Zn(r))return null;let s=[t.owner,t.name].map(encodeURIComponent).join("/"),i=r.split("/").map(encodeURIComponent).join("/");return`${e}/${s}/raw/${n}/${i}`}function Qn(e){return e?.payload?.pullRequestsChangesRoute??e?.pullRequestsChangesRoute??null}function Jn(e,t){let n=Qn(e);if(!n)return null;let r=n.comparison?.fullDiff??n.pullRequest?.comparison??{},s=r.baseOid,i=r.headOid,a=n.pullRequest?.headRepositoryOwnerLogin,o=n.pullRequest?.headRepositoryName,c=a&&o?{owner:a,name:o}:t,u=new Map((n.diffContents??[]).map(h=>[h.pathDigest,h])),l=n.diffSummaries?.length?n.diffSummaries:n.diffContents??[],p=new Map;for(let h of l){let d=u.get(h.pathDigest),f=d?.path??h.path,g=d&&Object.hasOwn(d,"oldTreeEntry")?d.oldTreeEntry?.path??null:f,b=d&&Object.hasOwn(d,"newTreeEntry")?d.newTreeEntry?.path??null:f;p.set(h.pathDigest,{digest:h.pathDigest,path:f,oldOid:d?.oldCommitOid??s,newOid:d?.newCommitOid??i,oldPath:g,newPath:b,oldRepository:t,newRepository:c})}return{baseOid:s,headOid:i,files:p}}function Kn(e,t){for(let n of e.querySelectorAll('script[type="application/json"]'))try{let r=JSON.parse(n.textContent),s=Jn(r,t);if(s)return s}catch{}return null}function Yn(e){return e?.replace(qn,"").trim()??""}function ge(e){let t=e.querySelector('h3 a[href^="#diff-"] code, h3 a[href^="#diff-"]'),n=Yn(t?.textContent);return n||(e.querySelector("[data-file-path]")?.getAttribute("data-file-path")??"")}function Me(e,t,n){if(!e||t.length===0||e.dataset.typerbSource===n)return;let r=e.ownerDocument,s=t.map(i=>{if(!i.className)return r.createTextNode(i.text);let a=r.createElement("span");return a.className=i.className,a.textContent=i.text,a});e.replaceChildren(...s),e.dataset.typerbSource=n}function er(e,t,n){if(!e||e.dataset.typerbSource===t)return;let r=e.ownerDocument,s=[];for(let i=0;i<n.length;i+=1){i>0&&s.push(r.createTextNode(`
`));for(let a of n[i]){if(!a.className){s.push(r.createTextNode(a.text));continue}let o=r.createElement("span");o.className=a.className,o.textContent=a.text,s.push(o)}}e.replaceChildren(...s),e.dataset.typerbSource=t}function tr(e,t,n){if(!t.pathname.includes("/blob/")||!z(decodeURIComponent(t.pathname)))return;let r=e.querySelector('[data-testid="read-only-cursor-text-area"]')?.value;if(typeof r!="string")return;let s=n.tokenizeSource(r);for(let i of e.querySelectorAll('.react-file-line[id^="LC"]')){let a=Number.parseInt(i.id.slice(2),10),o=s[a-1];o&&Me(i,o,i.textContent)}}function nr(e){let t=e.closest('[role="region"][id^="diff-"]');if(t&&z(ge(t)))return!0;let n=e.closest("[data-file-path], [data-path]");return z(n?.getAttribute("data-file-path")??n?.getAttribute("data-path"))}function rr(e,t){let n=e.querySelectorAll('pre[lang="trb"] > code, pre[lang="typerb"] > code, pre[lang="suggestion"] > code');for(let r of n){if(r.parentElement?.getAttribute("lang")?.toLowerCase()==="suggestion"&&!nr(r))continue;let i=r.textContent;er(r,i,t.tokenizeSource(i))}}async function mt({origin:e,repository:t,oid:n,path:r,highlighter:s,cache:i,fetchImpl:a}){let o=Xn(e,t,n,r);return o?(i.has(o)||(i.size>=64&&i.delete(i.keys().next().value),i.set(o,(async()=>{let c=await a(o,{credentials:"include"});if(!c.ok||c.headers.get("content-type")?.includes("text/html"))return null;let u=await c.text();return u.length>2e6?null:s.tokenizeSource(u)})().catch(()=>null))),i.get(o)):null}async function bt({side:e,file:t,origin:n,repository:r,highlighter:s,cache:i,fetchImpl:a}){let o=e==="left",c=o?t.oldOid:t.newOid,u=o?t.oldPath:t.newPath,l=o?t.oldRepository:t.newRepository,p=await mt({origin:n,repository:l,oid:c,path:u,highlighter:s,cache:i,fetchImpl:a});return p||o||l.owner===r.owner&&l.name===r.name?p:mt({origin:n,repository:r,oid:c,path:u,highlighter:s,cache:i,fetchImpl:a})}function sr(e,t){let n={left:{line:null,ruleStack:t.initialState()},right:{line:null,ruleStack:t.initialState()}};for(let r of e.querySelectorAll(".diff-text-cell[data-line-number][data-diff-side]")){let s=r.getAttribute("data-diff-side"),i=r.querySelector(".diff-text-inner"),a=Number.parseInt(r.getAttribute("data-line-number"),10);if(!i||!n[s]||!Number.isFinite(a)||i.dataset.typerbSource===i.textContent)continue;n[s].line!==a-1&&(n[s].ruleStack=t.initialState());let o=i.textContent,c=t.tokenizeLine(o,n[s].ruleStack);Me(i,c.segments,o),n[s]={line:a,ruleStack:c.ruleStack}}}async function ir({region:e,file:t,repository:n,origin:r,highlighter:s,cache:i,fetchImpl:a}){let[o,c]=await Promise.all([bt({side:"left",file:t,origin:r,repository:n,highlighter:s,cache:i,fetchImpl:a}),bt({side:"right",file:t,origin:r,repository:n,highlighter:s,cache:i,fetchImpl:a})]);for(let u of e.querySelectorAll(".diff-text-cell[data-line-number][data-diff-side]")){let l=u.querySelector(".diff-text-inner"),p=Number.parseInt(u.getAttribute("data-line-number"),10),d=(u.getAttribute("data-diff-side")==="left"?o:c)?.[p-1];l&&d&&Me(l,d,l.textContent)}sr(e,s)}async function ar(e,t,n,r,s=fetch){if(!/\/pull\/\d+\/(?:files|changes)(?:\/|$)/.test(t.pathname))return;let i=Vn(t.pathname);if(!i)return;let a=Kn(e,i)??{baseOid:null,headOid:null,files:new Map},o=[];for(let c of e.querySelectorAll('[role="region"][id^="diff-"]')){let u=c.id.slice(5),l=a.files.get(u)??{digest:u,path:ge(c),oldOid:a.baseOid,newOid:a.headOid,oldPath:ge(c),newPath:ge(c),oldRepository:i,newRepository:i};!z(l.path)&&!z(l.oldPath)&&!z(l.newPath)||o.push(ir({region:c,file:l,repository:i,origin:t.origin,highlighter:n,cache:r,fetchImpl:s}))}await Promise.all(o)}async function yt({document:e,location:t,highlighter:n,cache:r,fetchImpl:s}){tr(e,t,n),rr(e,n),await ar(e,t,n,r,s)}var wt=class{patterns;options;regexps;constructor(e,t={}){this.patterns=e,this.options=t;let{forgiving:n=!1,cache:r,regexConstructor:s}=t;if(!s)throw new Error("Option `regexConstructor` is not provided");this.regexps=e.map(i=>{if(typeof i!="string")return i;let a=r?.get(i);if(a){if(a instanceof RegExp)return a;if(n)return null;throw a}try{let o=s(i);return r?.set(i,o),o}catch(o){if(r?.set(i,o),n)return null;throw o}})}findNextMatchSync(e,t,n){let r=typeof e=="string"?e:e.content,s=[];function i(a,o,c=0){return{index:a,captureIndices:o.indices.map(u=>u==null?{start:4294967295,end:4294967295,length:0}:{start:u[0]+c,end:u[1]+c,length:u[1]-u[0]})}}for(let a=0;a<this.regexps.length;a++){let o=this.regexps[a];if(o)try{o.lastIndex=t;let c=o.exec(r);if(!c)continue;if(c.index===t)return i(a,c,0);s.push([a,c,0])}catch(c){if(this.options.forgiving)continue;throw c}}if(s.length){let a=Math.min(...s.map(o=>o[1].index));for(let[o,c,u]of s)if(c.index===a)return i(o,c,u)}return null}};function D(e){if([...e].length!==1)throw new Error(`Expected "${e}" to be a single code point`);return e.codePointAt(0)}function Ct(e,t,n){return e.has(t)||e.set(t,n),e.get(t)}var re=new Set(["alnum","alpha","ascii","blank","cntrl","digit","graph","lower","print","punct","space","upper","word","xdigit"]),x=String.raw;function G(e,t){if(e==null)throw new Error(t??"Value expected");return e}var It=x`\[\^?`,Rt=`c.? | C(?:-.?)?|${x`[pP]\{(?:\^?[-\x20_]*[A-Za-z][-\x20\w]*\})?`}|${x`x[89A-Fa-f]\p{AHex}(?:\\x[89A-Fa-f]\p{AHex})*`}|${x`u(?:\p{AHex}{4})? | x\{[^\}]*\}? | x\p{AHex}{0,2}`}|${x`o\{[^\}]*\}?`}|${x`\d{1,3}`}`,Oe=/[?*+][?+]?|\{(?:\d+(?:,\d*)?|,\d+)\}\??/,me=new RegExp(x`
  \\ (?:
    ${Rt}
    | [gk]<[^>]*>?
    | [gk]'[^']*'?
    | .
  )
  | \( (?:
    \? (?:
      [:=!>({]
      | <[=!]
      | <[^>]*>
      | '[^']*'
      | ~\|?
      | #(?:[^)\\]|\\.?)*
      | [^:)]*[:)]
    )?
    | \*[^\)]*\)?
  )?
  | (?:${Oe.source})+
  | ${It}
  | .
`.replace(/\s+/g,""),"gsu"),Te=new RegExp(x`
  \\ (?:
    ${Rt}
    | .
  )
  | \[:(?:\^?\p{Alpha}+|\^):\]
  | ${It}
  | &&
  | .
`.replace(/\s+/g,""),"gsu");function Et(e,t={}){let n={flags:"",...t,rules:{captureGroup:!1,singleline:!1,...t.rules}};if(typeof e!="string")throw new Error("String expected as pattern");let r=Ar(n.flags),s=[r.extended],i={captureGroup:n.rules.captureGroup,getCurrentModX(){return s.at(-1)},numOpenGroups:0,popModX(){s.pop()},pushModX(p){s.push(p)},replaceCurrentModX(p){s[s.length-1]=p},singleline:n.rules.singleline},a=[],o;for(me.lastIndex=0;o=me.exec(e);){let p=or(i,e,o[0],me.lastIndex);p.tokens?a.push(...p.tokens):p.token&&a.push(p.token),p.lastIndex!==void 0&&(me.lastIndex=p.lastIndex)}let c=[],u=0;a.filter(p=>p.type==="GroupOpen").forEach(p=>{p.kind==="capturing"?p.number=++u:p.raw==="("&&c.push(p)}),u||c.forEach((p,h)=>{p.kind="capturing",p.number=h+1});let l=u||c.length;return{tokens:a.map(p=>p.type==="EscapedNumber"?Rr(p,l):p).flat(),flags:r}}function or(e,t,n,r){let[s,i]=n;if(n==="["||n==="[^"){let a=cr(t,n,r);return{tokens:a.tokens,lastIndex:a.lastIndex}}if(s==="\\"){if("AbBGyYzZ".includes(i))return{token:_t(n,n)};if(/^\\g[<']/.test(n)){if(!/^\\g(?:<[^>]+>|'[^']+')$/.test(n))throw new Error(`Invalid group name "${n}"`);return{token:yr(n)}}if(/^\\k[<']/.test(n)){if(!/^\\k(?:<[^>]+>|'[^']+')$/.test(n))throw new Error(`Invalid group name "${n}"`);return{token:Nt(n)}}if(i==="K")return{token:Pt("keep",n)};if(i==="N"||i==="R")return{token:q("newline",n,{negate:i==="N"})};if(i==="O")return{token:q("any",n)};if(i==="X")return{token:q("text_segment",n)};let a=vt(n,{inCharClass:!1});return Array.isArray(a)?{tokens:a}:{token:a}}if(s==="("){if(i==="*")return{token:Sr(n)};if(n==="(?{")throw new Error(`Unsupported callout "${n}"`);if(n.startsWith("(?#")){if(t[r]!==")")throw new Error('Unclosed comment group "(?#"');return{lastIndex:r+1}}if(/^\(\?[-imx]+[:)]$/.test(n))return{token:_r(n,e)};if(e.pushModX(e.getCurrentModX()),e.numOpenGroups++,n==="("&&!e.captureGroup||n==="(?:")return{token:J("group",n)};if(n==="(?>")return{token:J("atomic",n)};if(n==="(?="||n==="(?!"||n==="(?<="||n==="(?<!")return{token:J(n[2]==="<"?"lookbehind":"lookahead",n,{negate:n.endsWith("!")})};if(n==="("&&e.captureGroup||n.startsWith("(?<")&&n.endsWith(">")||n.startsWith("(?'")&&n.endsWith("'"))return{token:J("capturing",n,{...n!=="("&&{name:n.slice(3,-1)}})};if(n.startsWith("(?~")){if(n==="(?~|")throw new Error(`Unsupported absence function kind "${n}"`);return{token:J("absence_repeater",n)}}throw n==="(?("?new Error(`Unsupported conditional "${n}"`):new Error(`Invalid or unsupported group option "${n}"`)}if(n===")"){if(e.popModX(),e.numOpenGroups--,e.numOpenGroups<0)throw new Error('Unmatched ")"');return{token:gr(n)}}if(e.getCurrentModX()){if(n==="#"){let a=t.indexOf(`
`,r);return{lastIndex:a===-1?t.length:a}}if(/^\s$/.test(n)){let a=/\s+/y;return a.lastIndex=r,{lastIndex:a.exec(t)?a.lastIndex:r}}}if(n===".")return{token:q("dot",n)};if(n==="^"||n==="$"){let a=e.singleline?{"^":x`\A`,$:x`\Z`}[n]:n;return{token:_t(a,n)}}return n==="|"?{token:lr(n)}:Oe.test(n)?{tokens:Er(n)}:{token:M(D(n),n)}}function cr(e,t,n){let r=[St(t[1]==="^",t)],s=1,i;for(Te.lastIndex=n;i=Te.exec(e);){let a=i[0];if(a[0]==="["&&a[1]!==":")s++,r.push(St(a[1]==="^",a));else if(a==="]"){if(r.at(-1).type==="CharacterClassOpen")r.push(M(93,a));else if(s--,r.push(pr(a)),!s)break}else{let o=ur(a);Array.isArray(o)?r.push(...o):r.push(o)}}return{tokens:r,lastIndex:Te.lastIndex||e.length}}function ur(e){if(e[0]==="\\")return vt(e,{inCharClass:!0});if(e[0]==="["){let t=/\[:(?<negate>\^?)(?<name>[a-z]+):\]/.exec(e);if(!t||!re.has(t.groups.name))throw new Error(`Invalid POSIX class "${e}"`);return q("posix",e,{value:t.groups.name,negate:!!t.groups.negate})}return e==="-"?hr(e):e==="&&"?dr(e):M(D(e),e)}function vt(e,{inCharClass:t}){let n=e[1];if(n==="c"||n==="C")return Cr(e);if("dDhHsSwW".includes(n))return kr(e);if(e.startsWith(x`\o{`))throw new Error(`Incomplete, invalid, or unsupported octal code point "${e}"`);if(/^\\[pP]\{/.test(e)){if(e.length===3)throw new Error(`Incomplete or invalid Unicode property "${e}"`);return xr(e)}if(new RegExp("^\\\\x[89A-Fa-f]\\p{AHex}","u").test(e))try{let r=e.split(/\\x/).slice(1).map(a=>parseInt(a,16)),s=new TextDecoder("utf-8",{ignoreBOM:!0,fatal:!0}).decode(new Uint8Array(r)),i=new TextEncoder;return[...s].map(a=>{let o=[...i.encode(a)].map(c=>`\\x${c.toString(16)}`).join("");return M(D(a),o)})}catch{throw new Error(`Multibyte code "${e}" incomplete or invalid in Oniguruma`)}if(n==="u"||n==="x")return M(Ir(e),e);if(kt.has(n))return M(kt.get(n),e);if(/\d/.test(n))return fr(t,e);if(e==="\\")throw new Error(x`Incomplete escape "\"`);if(n==="M")throw new Error(`Unsupported meta "${e}"`);if([...e].length===2)return M(e.codePointAt(1),e);throw new Error(`Unexpected escape "${e}"`)}function lr(e){return{type:"Alternator",raw:e}}function _t(e,t){return{type:"Assertion",kind:e,raw:t}}function Nt(e){return{type:"Backreference",raw:e}}function M(e,t){return{type:"Character",value:e,raw:t}}function pr(e){return{type:"CharacterClassClose",raw:e}}function hr(e){return{type:"CharacterClassHyphen",raw:e}}function dr(e){return{type:"CharacterClassIntersector",raw:e}}function St(e,t){return{type:"CharacterClassOpen",negate:e,raw:t}}function q(e,t,n={}){return{type:"CharacterSet",kind:e,...n,raw:t}}function Pt(e,t,n={}){return e==="keep"?{type:"Directive",kind:e,raw:t}:{type:"Directive",kind:e,flags:G(n.flags),raw:t}}function fr(e,t){return{type:"EscapedNumber",inCharClass:e,raw:t}}function gr(e){return{type:"GroupClose",raw:e}}function J(e,t,n={}){return{type:"GroupOpen",kind:e,...n,raw:t}}function mr(e,t,n,r){return{type:"NamedCallout",kind:e,tag:t,arguments:n,raw:r}}function br(e,t,n,r){return{type:"Quantifier",kind:e,min:t,max:n,raw:r}}function yr(e){return{type:"Subroutine",raw:e}}var wr=new Set(["COUNT","CMP","ERROR","FAIL","MAX","MISMATCH","SKIP","TOTAL_COUNT"]),kt=new Map([["a",7],["b",8],["e",27],["f",12],["n",10],["r",13],["t",9],["v",11]]);function Cr(e){let t=e[1]==="c"?e[2]:e[3];if(!t||!/[A-Za-z]/.test(t))throw new Error(`Unsupported control character "${e}"`);return M(D(t.toUpperCase())-64,e)}function _r(e,t){let{on:n,off:r}=/^\(\?(?<on>[imx]*)(?:-(?<off>[-imx]*))?/.exec(e).groups;r??="";let s=(t.getCurrentModX()||n.includes("x"))&&!r.includes("x"),i=At(n),a=At(r),o={};if(i&&(o.enable=i),a&&(o.disable=a),e.endsWith(")"))return t.replaceCurrentModX(s),Pt("flags",e,{flags:o});if(e.endsWith(":"))return t.pushModX(s),t.numOpenGroups++,J("group",e,{...(i||a)&&{flags:o}});throw new Error(`Unexpected flag modifier "${e}"`)}function Sr(e){let t=/\(\*(?<name>[A-Za-z_]\w*)?(?:\[(?<tag>(?:[A-Za-z_]\w*)?)\])?(?:\{(?<args>[^}]*)\})?\)/.exec(e);if(!t)throw new Error(`Incomplete or invalid named callout "${e}"`);let{name:n,tag:r,args:s}=t.groups;if(!n)throw new Error(`Invalid named callout "${e}"`);if(r==="")throw new Error(`Named callout tag with empty value not allowed "${e}"`);let i=s?s.split(",").filter(l=>l!=="").map(l=>/^[+-]?\d+$/.test(l)?+l:l):[],[a,o,c]=i,u=wr.has(n)?n.toLowerCase():"custom";switch(u){case"fail":case"mismatch":case"skip":if(i.length>0)throw new Error(`Named callout arguments not allowed "${i}"`);break;case"error":if(i.length>1)throw new Error(`Named callout allows only one argument "${i}"`);if(typeof a=="string")throw new Error(`Named callout argument must be a number "${a}"`);break;case"max":if(!i.length||i.length>2)throw new Error(`Named callout must have one or two arguments "${i}"`);if(typeof a=="string"&&!/^[A-Za-z_]\w*$/.test(a))throw new Error(`Named callout argument one must be a tag or number "${a}"`);if(i.length===2&&(typeof o=="number"||!/^[<>X]$/.test(o)))throw new Error(`Named callout optional argument two must be '<', '>', or 'X' "${o}"`);break;case"count":case"total_count":if(i.length>1)throw new Error(`Named callout allows only one argument "${i}"`);if(i.length===1&&(typeof a=="number"||!/^[<>X]$/.test(a)))throw new Error(`Named callout optional argument must be '<', '>', or 'X' "${a}"`);break;case"cmp":if(i.length!==3)throw new Error(`Named callout must have three arguments "${i}"`);if(typeof a=="string"&&!/^[A-Za-z_]\w*$/.test(a))throw new Error(`Named callout argument one must be a tag or number "${a}"`);if(typeof o=="number"||!/^(?:[<>!=]=|[<>])$/.test(o))throw new Error(`Named callout argument two must be '==', '!=', '>', '<', '>=', or '<=' "${o}"`);if(typeof c=="string"&&!/^[A-Za-z_]\w*$/.test(c))throw new Error(`Named callout argument three must be a tag or number "${c}"`);break;case"custom":throw new Error(`Undefined callout name "${n}"`);default:throw new Error(`Unexpected named callout kind "${u}"`)}return mr(u,r??null,s?.split(",")??null,e)}function xt(e){let t=null,n,r;if(e[0]==="{"){let{minStr:s,maxStr:i}=/^\{(?<minStr>\d*)(?:,(?<maxStr>\d*))?/.exec(e).groups,a=1e5;if(+s>a||i&&+i>a)throw new Error("Quantifier value unsupported in Oniguruma");if(n=+s,r=i===void 0?+s:i===""?1/0:+i,n>r&&(t="possessive",[n,r]=[r,n]),e.endsWith("?")){if(t==="possessive")throw new Error('Unsupported possessive interval quantifier chain with "?"');t="lazy"}else t||(t="greedy")}else n=e[0]==="+"?1:0,r=e[0]==="?"?1:1/0,t=e[1]==="+"?"possessive":e[1]==="?"?"lazy":"greedy";return br(t,n,r,e)}function kr(e){let t=e[1].toLowerCase();return q({d:"digit",h:"hex",s:"space",w:"word"}[t],e,{negate:e[1]!==t})}function xr(e){let{p:t,neg:n,value:r}=/^\\(?<p>[pP])\{(?<neg>\^?)(?<value>[^}]+)/.exec(e).groups;return q("property",e,{value:r,negate:t==="P"&&!n||t==="p"&&!!n})}function At(e){let t={};return e.includes("i")&&(t.ignoreCase=!0),e.includes("m")&&(t.dotAll=!0),e.includes("x")&&(t.extended=!0),Object.keys(t).length?t:null}function Ar(e){let t={ignoreCase:!1,dotAll:!1,extended:!1,digitIsAscii:!1,posixIsAscii:!1,spaceIsAscii:!1,wordIsAscii:!1,textSegmentMode:null};for(let n=0;n<e.length;n++){let r=e[n];if(!"imxDPSWy".includes(r))throw new Error(`Invalid flag "${r}"`);if(r==="y"){if(!/^y{[gw]}/.test(e.slice(n)))throw new Error('Invalid or unspecified flag "y" mode');t.textSegmentMode=e[n+2]==="g"?"grapheme":"word",n+=3;continue}t[{i:"ignoreCase",m:"dotAll",x:"extended",D:"digitIsAscii",P:"posixIsAscii",S:"spaceIsAscii",W:"wordIsAscii"}[r]]=!0}return t}function Ir(e){if(new RegExp("^(?:\\\\u(?!\\p{AHex}{4})|\\\\x(?!\\p{AHex}{1,2}|\\{\\p{AHex}{1,8}\\}))","u").test(e))throw new Error(`Incomplete or invalid escape "${e}"`);let t=e[2]==="{"?new RegExp("^\\\\x\\{\\s*(?<hex>\\p{AHex}+)","u").exec(e).groups.hex:e.slice(2);return parseInt(t,16)}function Rr(e,t){let{raw:n,inCharClass:r}=e,s=n.slice(1);if(!r&&(s!=="0"&&s.length===1||s[0]!=="0"&&+s<=t))return[Nt(n)];let i=[],a=s.match(/^[0-7]+|\d/g);for(let o=0;o<a.length;o++){let c=a[o],u;if(o===0&&c!=="8"&&c!=="9"){if(u=parseInt(c,8),u>127)throw new Error(x`Octal encoded byte above 177 unsupported "${n}"`)}else u=D(c);i.push(M(u,(o===0?"\\":"")+c))}return i}function Er(e){let t=[],n=new RegExp(Oe,"gy"),r;for(;r=n.exec(e);){let s=r[0];if(s[0]==="{"){let i=/^\{(?<min>\d+),(?<max>\d+)\}\??$/.exec(s);if(i){let{min:a,max:o}=i.groups;if(+a>+o&&s.endsWith("?")){n.lastIndex--,t.push(xt(s.slice(0,-1)));continue}}}t.push(xt(s))}return t}function be(e,t){if(!Array.isArray(e.body))throw new Error("Expected node with body array");if(e.body.length!==1)return!1;let n=e.body[0];return!t||Object.keys(t).every(r=>t[r]===n[r])}function $t(e){return vr.has(e.type)}var vr=new Set(["AbsenceFunction","Backreference","CapturingGroup","Character","CharacterClass","CharacterSet","Group","Quantifier","Subroutine"]);function we(e,t={}){let n={flags:"",normalizeUnknownPropertyNames:!1,skipBackrefValidation:!1,skipLookbehindValidation:!1,skipPropertyNameValidation:!1,unicodePropertyMap:null,...t,rules:{captureGroup:!1,singleline:!1,...t.rules}},r=Et(e,{flags:n.flags,rules:{captureGroup:n.rules.captureGroup,singleline:n.rules.singleline}}),s=(h,d)=>{let f=r.tokens[i.nextIndex];switch(i.parent=h,i.nextIndex++,f.type){case"Alternator":return T();case"Assertion":return Nr(f);case"Backreference":return Pr(f,i);case"Character":return K(f.value,{useLastValid:!!d.isCheckingRangeEnd});case"CharacterClassHyphen":return $r(f,i,d);case"CharacterClassOpen":return Lr(f,i,d);case"CharacterSet":return Gr(f,i);case"Directive":return Dr(f.kind,{flags:f.flags});case"GroupOpen":return Mr(f,i,d);case"NamedCallout":return Wr(f.kind,f.tag,f.arguments);case"Quantifier":return Tr(f,i);case"Subroutine":return Or(f,i);default:throw new Error(`Unexpected token type "${f.type}"`)}},i={capturingGroups:[],hasNumberedRef:!1,namedGroupsByName:new Map,nextIndex:0,normalizeUnknownPropertyNames:n.normalizeUnknownPropertyNames,parent:null,skipBackrefValidation:n.skipBackrefValidation,skipLookbehindValidation:n.skipLookbehindValidation,skipPropertyNameValidation:n.skipPropertyNameValidation,subroutines:[],tokens:r.tokens,unicodePropertyMap:n.unicodePropertyMap,walk:s},a=zr(Ur(r.flags)),o=a.body[0];for(;i.nextIndex<r.tokens.length;){let h=s(o,{});h.type==="Alternative"?(a.body.push(h),o=h):o.body.push(h)}let{capturingGroups:c,hasNumberedRef:u,namedGroupsByName:l,subroutines:p}=i;if(u&&l.size&&!n.rules.captureGroup)throw new Error("Numbered backref/subroutine not allowed when using named capture");for(let{ref:h}of p)if(typeof h=="number"){if(h>c.length)throw new Error("Subroutine uses a group number that's not defined");h&&(c[h-1].isSubroutined=!0)}else if(l.has(h)){if(l.get(h).length>1)throw new Error(x`Subroutine uses a duplicate group name "\g<${h}>"`);l.get(h)[0].isSubroutined=!0}else throw new Error(x`Subroutine uses a group name that's not defined "\g<${h}>"`);return a}function Nr({kind:e}){return Ce(G({"^":"line_start",$:"line_end","\\A":"string_start","\\b":"word_boundary","\\B":"word_boundary","\\G":"search_start","\\y":"text_segment_boundary","\\Y":"text_segment_boundary","\\z":"string_end","\\Z":"string_end_newline"}[e],`Unexpected assertion kind "${e}"`),{negate:e===x`\B`||e===x`\Y`})}function Pr({raw:e},t){let n=/^\\k[<']/.test(e),r=n?e.slice(3,-1):e.slice(1),s=(i,a=!1)=>{let o=t.capturingGroups.length,c=!1;if(i>o)if(t.skipBackrefValidation)c=!0;else throw new Error(`Not enough capturing groups defined to the left "${e}"`);return t.hasNumberedRef=!0,ye(a?o+1-i:i,{orphan:c})};if(n){let i=/^(?<sign>-?)0*(?<num>[1-9]\d*)$/.exec(r);if(i)return s(+i.groups.num,!!i.groups.sign);if(/[-+]/.test(r))throw new Error(`Invalid backref name "${e}"`);if(!t.namedGroupsByName.has(r))throw new Error(`Group name not defined to the left "${e}"`);return ye(r)}return s(+r)}function $r(e,t,n){let{tokens:r,walk:s}=t,i=t.parent,a=i.body.at(-1),o=r[t.nextIndex];if(!n.isCheckingRangeEnd&&a&&a.type!=="CharacterClass"&&a.type!=="CharacterClassRange"&&o&&o.type!=="CharacterClassOpen"&&o.type!=="CharacterClassClose"&&o.type!=="CharacterClassIntersector"){let c=s(i,{...n,isCheckingRangeEnd:!0});if(a.type==="Character"&&c.type==="Character")return i.body.pop(),Fr(a,c);throw new Error("Invalid character class range")}return K(D("-"))}function Lr({negate:e},t,n){let{tokens:r,walk:s}=t,i=[se()],a=r[t.nextIndex],o=Mt(a);for(;o.type!=="CharacterClassClose";){if(o.type==="CharacterClassIntersector")i.push(se()),t.nextIndex++;else{let u=i.at(-1);u.body.push(s(u,n))}o=Mt(r[t.nextIndex],a)}let c=se({negate:e});return i.length===1?c.body=i[0].body:(c.kind="intersection",c.body=i.map(u=>u.body.length===1?u.body[0]:u)),t.nextIndex++,c}function Gr({kind:e,negate:t,value:n},r){let{normalizeUnknownPropertyNames:s,skipPropertyNameValidation:i,unicodePropertyMap:a}=r;if(e==="property"){let o=Y(n);if(re.has(o)&&!a?.has(o))e="posix",n=o;else return H(n,{negate:t,normalizeUnknownPropertyNames:s,skipPropertyNameValidation:i,unicodePropertyMap:a})}return e==="posix"?jr(n,{negate:t}):_e(e,{negate:t})}function Mr(e,t,n){let{tokens:r,capturingGroups:s,namedGroupsByName:i,skipLookbehindValidation:a,walk:o}=t,c=qr(e),u=c.type==="AbsenceFunction",l=Gt(c),p=l&&c.negate;if(c.type==="CapturingGroup"&&(s.push(c),c.name&&Ct(i,c.name,[]).push(c)),u&&n.isInAbsenceFunction)throw new Error("Nested absence function not supported by Oniguruma");let h=Tt(r[t.nextIndex]);for(;h.type!=="GroupClose";){if(h.type==="Alternator")c.body.push(T()),t.nextIndex++;else{let d=c.body.at(-1),f=o(d,{...n,isInAbsenceFunction:n.isInAbsenceFunction||u,isInLookbehind:n.isInLookbehind||l,isInNegLookbehind:n.isInNegLookbehind||p});if(d.body.push(f),(l||n.isInLookbehind)&&!a){let g="Lookbehind includes a pattern not allowed by Oniguruma";if(p||n.isInNegLookbehind){if(Lt(f)||f.type==="CapturingGroup")throw new Error(g)}else if(Lt(f)||Gt(f)&&f.negate)throw new Error(g)}}h=Tt(r[t.nextIndex])}return t.nextIndex++,c}function Tr({kind:e,min:t,max:n},r){let s=r.parent,i=s.body.at(-1);if(!i||!$t(i))throw new Error("Quantifier requires a repeatable token");let a=Fe(e,t,n,i);return s.body.pop(),a}function Or({raw:e},t){let{capturingGroups:n,subroutines:r}=t,s=e.slice(3,-1),i=/^(?<sign>[-+]?)0*(?<num>[1-9]\d*)$/.exec(s);if(i){let o=+i.groups.num,c=n.length;if(t.hasNumberedRef=!0,s={"":o,"+":c+o,"-":c+1-o}[i.groups.sign],s<1)throw new Error("Invalid subroutine number")}else s==="0"&&(s=0);let a=De(s);return r.push(a),a}function Br(e,t){if(e!=="repeater")throw new Error(`Unexpected absence function kind "${e}"`);return{type:"AbsenceFunction",kind:e,body:ie(t?.body)}}function T(e){return{type:"Alternative",body:Ot(e?.body)}}function Ce(e,t){let n={type:"Assertion",kind:e};return(e==="word_boundary"||e==="text_segment_boundary")&&(n.negate=!!t?.negate),n}function ye(e,t){let n=!!t?.orphan;return{type:"Backreference",ref:e,...n&&{orphan:n}}}function Be(e,t){let n={name:void 0,isSubroutined:!1,...t};if(n.name!==void 0&&!Hr(n.name))throw new Error(`Group name "${n.name}" invalid in Oniguruma`);return{type:"CapturingGroup",number:e,...n.name&&{name:n.name},...n.isSubroutined&&{isSubroutined:n.isSubroutined},body:ie(t?.body)}}function K(e,t){let n={useLastValid:!1,...t};if(e>1114111){let r=e.toString(16);if(n.useLastValid)e=1114111;else throw e>1310719?new Error(`Invalid code point out of range "\\x{${r}}"`):new Error(`Invalid code point out of range in JS "\\x{${r}}"`)}return{type:"Character",value:e}}function se(e){let t={kind:"union",negate:!1,...e};return{type:"CharacterClass",kind:t.kind,negate:t.negate,body:Ot(e?.body)}}function Fr(e,t){if(t.value<e.value)throw new Error("Character class range out of order");return{type:"CharacterClassRange",min:e,max:t}}function _e(e,t){let n=!!t?.negate,r={type:"CharacterSet",kind:e};return(e==="digit"||e==="hex"||e==="newline"||e==="space"||e==="word")&&(r.negate=n),(e==="text_segment"||e==="newline"&&!n)&&(r.variableLength=!0),r}function Dr(e,t={}){if(e==="keep")return{type:"Directive",kind:e};if(e==="flags")return{type:"Directive",kind:e,flags:G(t.flags)};throw new Error(`Unexpected directive kind "${e}"`)}function Ur(e){return{type:"Flags",...e}}function E(e){let t=e?.atomic,n=e?.flags;if(t&&n)throw new Error("Atomic group cannot have flags");return{type:"Group",...t&&{atomic:t},...n&&{flags:n},body:ie(e?.body)}}function U(e){let t={behind:!1,negate:!1,...e};return{type:"LookaroundAssertion",kind:t.behind?"lookbehind":"lookahead",negate:t.negate,body:ie(e?.body)}}function Wr(e,t,n){return{type:"NamedCallout",kind:e,tag:t,arguments:n}}function jr(e,t){let n=!!t?.negate;if(!re.has(e))throw new Error(`Invalid POSIX class "${e}"`);return{type:"CharacterSet",kind:"posix",value:e,negate:n}}function Fe(e,t,n,r){if(t>n)throw new Error("Invalid reversed quantifier range");return{type:"Quantifier",kind:e,min:t,max:n,body:r}}function zr(e,t){return{type:"Regex",body:ie(t?.body),flags:e}}function De(e){return{type:"Subroutine",ref:e}}function H(e,t){let n={negate:!1,normalizeUnknownPropertyNames:!1,skipPropertyNameValidation:!1,unicodePropertyMap:null,...t},r=n.unicodePropertyMap?.get(Y(e));if(!r){if(n.normalizeUnknownPropertyNames)r=Vr(e);else if(n.unicodePropertyMap&&!n.skipPropertyNameValidation)throw new Error(x`Invalid Unicode property "\p{${e}}"`)}return{type:"CharacterSet",kind:"property",value:r??e,negate:n.negate}}function qr({flags:e,kind:t,name:n,negate:r,number:s}){switch(t){case"absence_repeater":return Br("repeater");case"atomic":return E({atomic:!0});case"capturing":return Be(s,{name:n});case"group":return E({flags:e});case"lookahead":case"lookbehind":return U({behind:t==="lookbehind",negate:r});default:throw new Error(`Unexpected group kind "${t}"`)}}function ie(e){if(e===void 0)e=[T()];else if(!Array.isArray(e)||!e.length||!e.every(t=>t.type==="Alternative"))throw new Error("Invalid body; expected array of one or more Alternative nodes");return e}function Ot(e){if(e===void 0)e=[];else if(!Array.isArray(e)||!e.every(t=>!!t.type))throw new Error("Invalid body; expected array of nodes");return e}function Lt(e){return e.type==="LookaroundAssertion"&&e.kind==="lookahead"}function Gt(e){return e.type==="LookaroundAssertion"&&e.kind==="lookbehind"}function Hr(e){return/^[\p{Alpha}\p{Pc}][^)]*$/u.test(e)}function Vr(e){return e.trim().replace(/[- _]+/g,"_").replace(/[A-Z][a-z]+(?=[A-Z])/g,"$&_").replace(/[A-Za-z]+/g,t=>t[0].toUpperCase()+t.slice(1).toLowerCase())}function Y(e){return e.replace(/[- _]+/g,"").toLowerCase()}function Mt(e,t){let n=t;return G(e,`Unclosed character class${n?.type==="Character"&&n.value===93&&n.raw==="]"?' (started with "]")':""}`)}function Tt(e){return G(e,"Unclosed group")}function V(e,t,n=null){function r(i,a){for(let o=0;o<i.length;o++){let c=s(i[o],a,o,i);o=Math.max(-1,o+c)}}function s(i,a=null,o=null,c=null){let u=0,l=!1,p={node:i,parent:a,key:o,container:c,root:e,remove(){Se(c).splice(Math.max(0,ee(o)+u),1),u--,l=!0},removeAllNextSiblings(){return Se(c).splice(ee(o)+1)},removeAllPrevSiblings(){let y=ee(o)+u;return u-=y,Se(c).splice(0,Math.max(0,y))},replaceWith(y,w={}){let k=!!w.traverse;c?c[Math.max(0,ee(o)+u)]=y:G(a,"Can't replace root node")[o]=y,k&&s(y,a,o,c),l=!0},replaceWithMultiple(y,w={}){let k=!!w.traverse;if(Se(c).splice(Math.max(0,ee(o)+u),1,...y),u+=y.length-1,k){let C=0;for(let _=0;_<y.length;_++)C+=s(y[_],a,ee(o)+_+C,c)}l=!0},skip(){l=!0}},{type:h}=i,d=t["*"],f=t[h],g=typeof d=="function"?d:d?.enter,b=typeof f=="function"?f:f?.enter;if(g?.(p,n),b?.(p,n),!l)switch(h){case"AbsenceFunction":case"Alternative":case"CapturingGroup":case"CharacterClass":case"Group":case"LookaroundAssertion":r(i.body,i);break;case"Assertion":case"Backreference":case"Character":case"CharacterSet":case"Directive":case"Flags":case"NamedCallout":case"Subroutine":break;case"CharacterClassRange":s(i.min,i,"min"),s(i.max,i,"max");break;case"Quantifier":s(i.body,i,"body");break;case"Regex":r(i.body,i),s(i.flags,i,"flags");break;default:throw new Error(`Unexpected node type "${h}"`)}return f?.exit?.(p,n),d?.exit?.(p,n),u}return s(e),e}function Se(e){if(!Array.isArray(e))throw new Error("Container expected");return e}function ee(e){if(typeof e!="number")throw new Error("Numeric key expected");return e}var Bt=String.raw`\(\?(?:[:=!>A-Za-z\-]|<[=!]|\(DEFINE\))`;function Ft(e,t){for(let n=0;n<e.length;n++)e[n]>=t&&e[n]++}function Dt(e,t,n,r){return e.slice(0,t)+r+e.slice(t+n.length)}var R=Object.freeze({DEFAULT:"DEFAULT",CHAR_CLASS:"CHAR_CLASS"});function ae(e,t,n,r){let s=new RegExp(String.raw`${t}|(?<$skip>\[\^?|\\?.)`,"gsu"),i=[!1],a=0,o="";for(let c of e.matchAll(s)){let{0:u,groups:{$skip:l}}=c;if(!l&&(!r||r===R.DEFAULT==!a)){n instanceof Function?o+=n(c,{context:a?R.CHAR_CLASS:R.DEFAULT,negated:i[i.length-1]}):o+=n;continue}u[0]==="["?(a++,i.push(u[1]==="^")):u==="]"&&a&&(a--,i.pop()),o+=u}return o}function Ue(e,t,n,r){ae(e,t,n,r)}function Zr(e,t,n=0,r){if(!new RegExp(t,"su").test(e))return null;let s=new RegExp(`${t}|(?<$skip>\\\\?.)`,"gsu");s.lastIndex=n;let i=0,a;for(;a=s.exec(e);){let{0:o,groups:{$skip:c}}=a;if(!c&&(!r||r===R.DEFAULT==!i))return a;o==="["?i++:o==="]"&&i&&i--,s.lastIndex==a.index&&s.lastIndex++}return null}function oe(e,t,n){return!!Zr(e,t,0,n)}function Ut(e,t){let n=/\\?./gsu;n.lastIndex=t;let r=e.length,s=0,i=1,a;for(;a=n.exec(e);){let[o]=a;if(o==="[")s++;else if(s)o==="]"&&s--;else if(o==="(")i++;else if(o===")"&&(i--,!i)){r=a.index;break}}return e.slice(t,r)}var Wt=new RegExp(String.raw`(?<noncapturingStart>${Bt})|(?<capturingStart>\((?:\?<[^>]+>)?)|\\?.`,"gsu");function je(e,t){let n=t?.hiddenCaptures??[],r=t?.captureTransfers??new Map;if(!/\(\?>/.test(e))return{pattern:e,captureTransfers:r,hiddenCaptures:n};let s="(?>",i="(?:(?=(",a=[0],o=[],c=0,u=0,l=NaN,p;do{p=!1;let h=0,d=0,f=!1,g;for(Wt.lastIndex=Number.isNaN(l)?0:l+i.length;g=Wt.exec(e);){let{0:b,index:y,groups:{capturingStart:w,noncapturingStart:k}}=g;if(b==="[")h++;else if(h)b==="]"&&h--;else if(b===s&&!f)l=y,f=!0;else if(f&&k)d++;else if(w)f?d++:(c++,a.push(c+u));else if(b===")"&&f){if(!d){u++;let C=c+u;if(e=`${e.slice(0,l)}${i}${e.slice(l+s.length,y)}))<$$${C}>)${e.slice(y+1)}`,p=!0,o.push(C),Ft(n,C),r.size){let _=new Map;r.forEach((F,$)=>{_.set($>=C?$+1:$,F.map(Q=>Q>=C?Q+1:Q))}),r=_}break}d--}}}while(p);return n.push(...o),e=ae(e,String.raw`\\(?<backrefNum>[1-9]\d*)|<\$\$(?<wrappedBackrefNum>\d+)>`,({0:h,groups:{backrefNum:d,wrappedBackrefNum:f}})=>{if(d){let g=+d;if(g>a.length-1)throw new Error(`Backref "${h}" greater than number of captures`);return`\\${a[g]}`}return`\\${f}`},R.DEFAULT),{pattern:e,captureTransfers:r,hiddenCaptures:n}}var jt=String.raw`(?:[?*+]|\{\d+(?:,\d*)?\})`,We=new RegExp(String.raw`
\\(?: \d+
  | c[A-Za-z]
  | [gk]<[^>]+>
  | [pPu]\{[^\}]+\}
  | u[A-Fa-f\d]{4}
  | x[A-Fa-f\d]{2}
  )
| \((?: \? (?: [:=!>]
  | <(?:[=!]|[^>]+>)
  | [A-Za-z\-]+:
  | \(DEFINE\)
  ))?
| (?<qBase>${jt})(?<qMod>[?+]?)(?<invalidQ>[?*+\{]?)
| \\?.
`.replace(/\s+/g,""),"gsu");function ze(e){if(!new RegExp(`${jt}\\+`).test(e))return{pattern:e};let t=[],n=null,r=null,s="",i=0,a;for(We.lastIndex=0;a=We.exec(e);){let{0:o,index:c,groups:{qBase:u,qMod:l,invalidQ:p}}=a;if(o==="[")i||(r=c),i++;else if(o==="]")i?i--:r=null;else if(!i)if(l==="+"&&s&&!s.startsWith("(")){if(p)throw new Error(`Invalid quantifier "${o}"`);let h=-1;if(/^\{\d+\}$/.test(u))e=Dt(e,c+u.length,l,"");else{if(s===")"||s==="]"){let d=s===")"?n:r;if(d===null)throw new Error(`Invalid unmatched "${s}"`);e=`${e.slice(0,d)}(?>${e.slice(d,c)}${u})${e.slice(c+o.length)}`}else e=`${e.slice(0,c-s.length)}(?>${s}${u})${e.slice(c+o.length)}`;h+=4}We.lastIndex+=h}else o[0]==="("?t.push(c):o===")"&&(n=t.length?t.pop():null);s=o}return{pattern:e}}var v=String.raw,Xr=v`\\g<(?<gRNameOrNum>[^>&]+)&R=(?<gRDepth>[^>]+)>`,He=v`\(\?R=(?<rDepth>[^\)]+)\)|${Xr}`,ke=v`\(\?<(?![=!])(?<captureName>[^>]+)>`,Zt=v`${ke}|(?<unnamed>\()(?!\?)`,Z=new RegExp(v`${ke}|${He}|\(\?|\\?.`,"gsu"),qe="Cannot use multiple overlapping recursions";function Xt(e,t){let{hiddenCaptures:n,mode:r}={hiddenCaptures:[],mode:"plugin",...t},s=t?.captureTransfers??new Map;if(!new RegExp(He,"su").test(e))return{pattern:e,captureTransfers:s,hiddenCaptures:n};if(r==="plugin"&&oe(e,v`\(\?\(DEFINE\)`,R.DEFAULT))throw new Error("DEFINE groups cannot be used with recursion");let i=[],a=oe(e,v`\\[1-9]`,R.DEFAULT),o=new Map,c=[],u=!1,l=0,p=0,h;for(Z.lastIndex=0;h=Z.exec(e);){let{0:d,groups:{captureName:f,rDepth:g,gRNameOrNum:b,gRDepth:y}}=h;if(d==="[")l++;else if(l)d==="]"&&l--;else if(g){if(zt(g),u)throw new Error(qe);if(a)throw new Error(`${r==="external"?"Backrefs":"Numbered backrefs"} cannot be used with global recursion`);let w=e.slice(0,h.index),k=e.slice(Z.lastIndex);if(oe(k,He,R.DEFAULT))throw new Error(qe);let C=+g-1;e=qt(w,k,C,!1,n,i,p),s=Vt(s,w,C,i.length,0,p);break}else if(b){zt(y);let w=!1;for(let ne of c)if(ne.name===b||ne.num===+b){if(w=!0,ne.hasRecursedWithin)throw new Error(qe);break}if(!w)throw new Error(v`Recursive \g cannot be used outside the referenced group "${r==="external"?b:v`\g<${b}&R=${y}>`}"`);let k=o.get(b),C=Ut(e,k);if(a&&oe(C,v`${ke}|\((?!\?)`,R.DEFAULT))throw new Error(`${r==="external"?"Backrefs":"Numbered backrefs"} cannot be used with recursion of capturing groups`);let _=e.slice(k,h.index),F=C.slice(_.length+d.length),$=i.length,Q=+y-1,ft=qt(_,F,Q,!0,n,i,p);s=Vt(s,_,Q,i.length-$,$,p);let jn=e.slice(0,k),zn=e.slice(k+C.length);e=`${jn}${ft}${zn}`,Z.lastIndex+=ft.length-d.length-_.length-F.length,c.forEach(ne=>ne.hasRecursedWithin=!0),u=!0}else if(f)p++,o.set(String(p),Z.lastIndex),o.set(f,Z.lastIndex),c.push({num:p,name:f});else if(d[0]==="("){let w=d==="(";w&&(p++,o.set(String(p),Z.lastIndex)),c.push(w?{num:p}:{})}else d===")"&&c.pop()}return n.push(...i),{pattern:e,captureTransfers:s,hiddenCaptures:n}}function zt(e){let t=`Max depth must be integer between 2 and 100; used ${e}`;if(!/^[1-9]\d*$/.test(e))throw new Error(t);if(e=+e,e<2||e>100)throw new Error(t)}function qt(e,t,n,r,s,i,a){let o=new Set;r&&Ue(e+t,ke,({groups:{captureName:u}})=>{o.add(u)},R.DEFAULT);let c=[n,r?o:null,s,i,a];return`${e}${Ht(`(?:${e}`,"forward",...c)}(?:)${Ht(`${t})`,"backward",...c)}${t}`}function Ht(e,t,n,r,s,i,a){let c=l=>t==="forward"?l+2:n-l+2-1,u="";for(let l=0;l<n;l++){let p=c(l);u+=ae(e,v`${Zt}|\\k<(?<backref>[^>]+)>`,({0:h,groups:{captureName:d,unnamed:f,backref:g}})=>{if(g&&r&&!r.has(g))return h;let b=`_$${p}`;if(f||d){let y=a+i.length+1;return i.push(y),Qr(s,y),f?h:`(?<${d}${b}>`}return v`\k<${g}${b}>`},R.DEFAULT)}return u}function Qr(e,t){for(let n=0;n<e.length;n++)e[n]>=t&&e[n]++}function Vt(e,t,n,r,s,i){if(e.size&&r){let a=0;Ue(t,Zt,()=>a++,R.DEFAULT);let o=i-a+s,c=new Map;return e.forEach((u,l)=>{let p=(r-a*n)/n,h=a*n,d=l>o+a?l+r:l,f=[];for(let g of u)if(g<=o)f.push(g);else if(g>o+a+p)f.push(g+r);else if(g<=o+a)for(let b=0;b<=n;b++)f.push(g+a*b);else for(let b=0;b<=n;b++)f.push(g+h+p*b);c.set(d,f)}),c}return e}var A=String.fromCodePoint,m=String.raw,P={},Ae=globalThis.RegExp;P.flagGroups=(()=>{try{new Ae("(?i:)")}catch{return!1}return!0})();P.unicodeSets=(()=>{try{new Ae("[[]]","v")}catch{return!1}return!0})();P.bugFlagVLiteralHyphenIsRange=P.unicodeSets?(()=>{try{new Ae(m`[\d\-a]`,"v")}catch{return!0}return!1})():!1;P.bugNestedClassIgnoresNegation=P.unicodeSets&&new Ae("[[^a]]","v").test("a");function xe(e,{enable:t,disable:n}){return{dotAll:!n?.dotAll&&!!(t?.dotAll||e.dotAll),ignoreCase:!n?.ignoreCase&&!!(t?.ignoreCase||e.ignoreCase)}}function ce(e,t,n){return e.has(t)||e.set(t,n),e.get(t)}function Je(e,t){return Qt[e]>=Qt[t]}function Jr(e,t){if(e==null)throw new Error(t??"Value expected");return e}var Qt={ES2025:2025,ES2024:2024,ES2018:2018},Kr={auto:"auto",ES2025:"ES2025",ES2024:"ES2024",ES2018:"ES2018"};function tn(e={}){if({}.toString.call(e)!=="[object Object]")throw new Error("Unexpected options");if(e.target!==void 0&&!Kr[e.target])throw new Error(`Unexpected target "${e.target}"`);let t={accuracy:"default",avoidSubclass:!1,flags:"",global:!1,hasIndices:!1,lazyCompileLength:1/0,target:"auto",verbose:!1,...e,rules:{allowOrphanBackrefs:!1,asciiWordBoundaries:!1,captureGroup:!1,recursionLimit:20,singleline:!1,...e.rules}};return t.target==="auto"&&(t.target=P.flagGroups?"ES2025":P.unicodeSets?"ES2024":"ES2018"),t}var Yr="[	-\r ]",es=new Set([A(304),A(305)]),O=m`[\p{L}\p{M}\p{N}\p{Pc}]`;function nn(e){if(es.has(e))return[e];let t=new Set,n=e.toLowerCase(),r=n.toUpperCase(),s=rs.get(n),i=ts.get(n),a=ns.get(n);return[...r].length===1&&t.add(r),a&&t.add(a),s&&t.add(s),t.add(n),i&&t.add(i),[...t]}var Ye=new Map(`C Other
Cc Control cntrl
Cf Format
Cn Unassigned
Co Private_Use
Cs Surrogate
L Letter
LC Cased_Letter
Ll Lowercase_Letter
Lm Modifier_Letter
Lo Other_Letter
Lt Titlecase_Letter
Lu Uppercase_Letter
M Mark Combining_Mark
Mc Spacing_Mark
Me Enclosing_Mark
Mn Nonspacing_Mark
N Number
Nd Decimal_Number digit
Nl Letter_Number
No Other_Number
P Punctuation punct
Pc Connector_Punctuation
Pd Dash_Punctuation
Pe Close_Punctuation
Pf Final_Punctuation
Pi Initial_Punctuation
Po Other_Punctuation
Ps Open_Punctuation
S Symbol
Sc Currency_Symbol
Sk Modifier_Symbol
Sm Math_Symbol
So Other_Symbol
Z Separator
Zl Line_Separator
Zp Paragraph_Separator
Zs Space_Separator
ASCII
ASCII_Hex_Digit AHex
Alphabetic Alpha
Any
Assigned
Bidi_Control Bidi_C
Bidi_Mirrored Bidi_M
Case_Ignorable CI
Cased
Changes_When_Casefolded CWCF
Changes_When_Casemapped CWCM
Changes_When_Lowercased CWL
Changes_When_NFKC_Casefolded CWKCF
Changes_When_Titlecased CWT
Changes_When_Uppercased CWU
Dash
Default_Ignorable_Code_Point DI
Deprecated Dep
Diacritic Dia
Emoji
Emoji_Component EComp
Emoji_Modifier EMod
Emoji_Modifier_Base EBase
Emoji_Presentation EPres
Extended_Pictographic ExtPict
Extender Ext
Grapheme_Base Gr_Base
Grapheme_Extend Gr_Ext
Hex_Digit Hex
IDS_Binary_Operator IDSB
IDS_Trinary_Operator IDST
ID_Continue IDC
ID_Start IDS
Ideographic Ideo
Join_Control Join_C
Logical_Order_Exception LOE
Lowercase Lower
Math
Noncharacter_Code_Point NChar
Pattern_Syntax Pat_Syn
Pattern_White_Space Pat_WS
Quotation_Mark QMark
Radical
Regional_Indicator RI
Sentence_Terminal STerm
Soft_Dotted SD
Terminal_Punctuation Term
Unified_Ideograph UIdeo
Uppercase Upper
Variation_Selector VS
White_Space space
XID_Continue XIDC
XID_Start XIDS`.split(/\s/).map(e=>[Y(e),e])),ts=new Map([["s",A(383)],[A(383),"s"]]),ns=new Map([[A(223),A(7838)],[A(107),A(8490)],[A(229),A(8491)],[A(969),A(8486)]]),rs=new Map([W(453),W(456),W(459),W(498),...Ve(8072,8079),...Ve(8088,8095),...Ve(8104,8111),W(8124),W(8140),W(8188)]),ss=new Map([["alnum",m`[\p{Alpha}\p{Nd}]`],["alpha",m`\p{Alpha}`],["ascii",m`\p{ASCII}`],["blank",m`[\p{Zs}\t]`],["cntrl",m`\p{Cc}`],["digit",m`\p{Nd}`],["graph",m`[\P{space}&&\P{Cc}&&\P{Cn}&&\P{Cs}]`],["lower",m`\p{Lower}`],["print",m`[[\P{space}&&\P{Cc}&&\P{Cn}&&\P{Cs}]\p{Zs}]`],["punct",m`[\p{P}\p{S}]`],["space",m`\p{space}`],["upper",m`\p{Upper}`],["word",m`[\p{Alpha}\p{M}\p{Nd}\p{Pc}]`],["xdigit",m`\p{AHex}`]]);function is(e,t){let n=[];for(let r=e;r<=t;r++)n.push(r);return n}function W(e){let t=A(e);return[t.toLowerCase(),t]}function Ve(e,t){return is(e,t).map(n=>W(n))}var rn=new Set(["Lower","Lowercase","Upper","Uppercase","Ll","Lowercase_Letter","Lt","Titlecase_Letter","Lu","Uppercase_Letter"]);function as(e,t){let n={accuracy:"default",asciiWordBoundaries:!1,avoidSubclass:!1,bestEffortTarget:"ES2025",...t};sn(e);let r={accuracy:n.accuracy,asciiWordBoundaries:n.asciiWordBoundaries,avoidSubclass:n.avoidSubclass,flagDirectivesByAlt:new Map,jsGroupNameMap:new Map,minTargetEs2024:Je(n.bestEffortTarget,"ES2024"),passedLookbehind:!1,strategy:null,subroutineRefMap:new Map,supportedGNodes:new Set,digitIsAscii:e.flags.digitIsAscii,spaceIsAscii:e.flags.spaceIsAscii,wordIsAscii:e.flags.wordIsAscii};V(e,os,r);let s={dotAll:e.flags.dotAll,ignoreCase:e.flags.ignoreCase},i={currentFlags:s,prevFlags:null,globalFlags:s,groupOriginByCopy:new Map,groupsByName:new Map,multiplexCapturesToLeftByRef:new Map,openRefs:new Map,reffedNodesByReferencer:new Map,subroutineRefMap:r.subroutineRefMap};V(e,cs,i);let a={groupsByName:i.groupsByName,highestOrphanBackref:0,numCapturesToLeft:0,reffedNodesByReferencer:i.reffedNodesByReferencer};return V(e,us,a),e._originMap=i.groupOriginByCopy,e._strategy=r.strategy,e}var os={AbsenceFunction({node:e,parent:t,replaceWith:n}){let{body:r,kind:s}=e;if(s==="repeater"){let i=E();i.body[0].body.push(U({negate:!0,body:r}),H("Any"));let a=E();a.body[0].body.push(Fe("greedy",0,1/0,i)),n(S(a,t),{traverse:!0})}else throw new Error('Unsupported absence function "(?~|"')},Alternative:{enter({node:e,parent:t,key:n},{flagDirectivesByAlt:r}){let s=e.body.filter(i=>i.kind==="flags");for(let i=n+1;i<t.body.length;i++){let a=t.body[i];ce(r,a,[]).push(...s)}},exit({node:e},{flagDirectivesByAlt:t}){if(t.get(e)?.length){let n=on(t.get(e));if(n){let r=E({flags:n});r.body[0].body=e.body,e.body=[S(r,e)]}}}},Assertion({node:e,parent:t,key:n,container:r,root:s,remove:i,replaceWith:a},o){let{kind:c,negate:u}=e,{asciiWordBoundaries:l,avoidSubclass:p,supportedGNodes:h,wordIsAscii:d}=o;if(c==="text_segment_boundary")throw new Error(`Unsupported text segment boundary "\\${u?"Y":"y"}"`);if(c==="line_end")a(S(U({body:[T({body:[Ce("string_end")]}),T({body:[K(10)]})]}),t));else if(c==="line_start")a(S(B(m`(?<=\A|\n(?!\z))`,{skipLookbehindValidation:!0}),t));else if(c==="search_start")if(h.has(e))s.flags.sticky=!0,i();else{let f=r[n-1];if(f&&gs(f))a(S(U({negate:!0}),t));else{if(p)throw new Error(m`Uses "\G" in a way that requires a subclass`);a(j(Ce("string_start"),t)),o.strategy="clip_search"}}else if(!(c==="string_end"||c==="string_start"))if(c==="string_end_newline")a(S(B(m`(?=\n?\z)`),t));else if(c==="word_boundary"){if(!d&&!l){let f=`(?:(?<=${O})(?!${O})|(?<!${O})(?=${O}))`,g=`(?:(?<=${O})(?=${O})|(?<!${O})(?!${O}))`;a(S(B(u?g:f),t))}}else throw new Error(`Unexpected assertion kind "${c}"`)},Backreference({node:e},{jsGroupNameMap:t}){let{ref:n}=e;typeof n=="string"&&!Xe(n)&&(n=Ze(n,t),e.ref=n)},CapturingGroup({node:e},{jsGroupNameMap:t,subroutineRefMap:n}){let{name:r}=e;r&&!Xe(r)&&(r=Ze(r,t),e.name=r),n.set(e.number,e),r&&n.set(r,e)},CharacterClassRange({node:e,parent:t,replaceWith:n}){if(t.kind==="intersection"){let r=se({body:[e]});n(S(r,t),{traverse:!0})}},CharacterSet({node:e,parent:t,replaceWith:n},{accuracy:r,minTargetEs2024:s,digitIsAscii:i,spaceIsAscii:a,wordIsAscii:o}){let{kind:c,negate:u,value:l}=e;if(i&&(c==="digit"||l==="digit")){n(j(_e("digit",{negate:u}),t));return}if(a&&(c==="space"||l==="space")){n(S(Qe(B(Yr),u),t));return}if(o&&(c==="word"||l==="word")){n(j(_e("word",{negate:u}),t));return}if(c==="any")n(j(H("Any"),t));else if(c==="digit")n(j(H("Nd",{negate:u}),t));else if(c!=="dot")if(c==="text_segment"){if(r==="strict")throw new Error(m`Use of "\X" requires non-strict accuracy`);let p="\\p{Emoji}(?:\\p{EMod}|\\uFE0F\\u20E3?|[\\x{E0020}-\\x{E007E}]+\\x{E007F})?",h=m`\p{RI}{2}|${p}(?:\u200D${p})*`;n(S(B(m`(?>\r\n|${s?m`\p{RGI_Emoji}`:h}|\P{M}\p{M}*)`,{skipPropertyNameValidation:!0}),t))}else if(c==="hex")n(j(H("AHex",{negate:u}),t));else if(c==="newline")n(S(B(u?`[^
]`:`(?>\r
?|[
\v\f\u2028\u2029])`),t));else if(c==="posix")if(!s&&(l==="graph"||l==="print")){if(r==="strict")throw new Error(`POSIX class "${l}" requires min target ES2024 or non-strict accuracy`);let p={graph:"!-~",print:" -~"}[l];u&&(p=`\0-${A(p.codePointAt(0)-1)}${A(p.codePointAt(2)+1)}-􏿿`),n(S(B(`[${p}]`),t))}else n(S(Qe(B(ss.get(l)),u),t));else if(c==="property")Ye.has(Y(l))||(e.key="sc");else if(c==="space")n(j(H("space",{negate:u}),t));else if(c==="word")n(S(Qe(B(O),u),t));else throw new Error(`Unexpected character set kind "${c}"`)},Directive({node:e,parent:t,root:n,remove:r,replaceWith:s,removeAllPrevSiblings:i,removeAllNextSiblings:a}){let{kind:o,flags:c}=e;if(o==="flags")if(!c.enable&&!c.disable)r();else{let u=E({flags:c});u.body[0].body=a(),s(S(u,t),{traverse:!0})}else if(o==="keep"){let u=n.body[0],p=n.body.length===1&&be(u,{type:"Group"})&&u.body[0].body.length===1?u.body[0]:n;if(t.parent!==p||p.body.length>1)throw new Error(m`Uses "\K" in a way that's unsupported`);let h=U({behind:!0});h.body[0].body=i(),s(S(h,t))}else throw new Error(`Unexpected directive kind "${o}"`)},Flags({node:e,parent:t}){if(e.posixIsAscii)throw new Error('Unsupported flag "P"');if(e.textSegmentMode==="word")throw new Error('Unsupported flag "y{w}"');["digitIsAscii","extended","posixIsAscii","spaceIsAscii","wordIsAscii","textSegmentMode"].forEach(n=>delete e[n]),Object.assign(e,{global:!1,hasIndices:!1,multiline:!1,sticky:e.sticky??!1}),t.options={disable:{x:!0,n:!0},force:{v:!0}}},Group({node:e}){if(!e.flags)return;let{enable:t,disable:n}=e.flags;t?.extended&&delete t.extended,n?.extended&&delete n.extended,t?.dotAll&&n?.dotAll&&delete t.dotAll,t?.ignoreCase&&n?.ignoreCase&&delete t.ignoreCase,t&&!Object.keys(t).length&&delete e.flags.enable,n&&!Object.keys(n).length&&delete e.flags.disable,!e.flags.enable&&!e.flags.disable&&delete e.flags},LookaroundAssertion({node:e},t){let{kind:n}=e;n==="lookbehind"&&(t.passedLookbehind=!0)},NamedCallout({node:e,parent:t,replaceWith:n}){let{kind:r}=e;if(r==="fail")n(S(U({negate:!0}),t));else throw new Error(`Unsupported named callout "(*${r.toUpperCase()}"`)},Quantifier({node:e}){if(e.body.type==="Quantifier"){let t=E();t.body[0].body.push(e.body),e.body=S(t,e)}},Regex:{enter({node:e},{supportedGNodes:t}){let n=[],r=!1,s=!1;for(let i of e.body)if(i.body.length===1&&i.body[0].kind==="search_start")i.body.pop();else{let a=un(i.body);a?(r=!0,Array.isArray(a)?n.push(...a):n.push(a)):s=!0}r&&!s&&n.forEach(i=>t.add(i))},exit(e,{accuracy:t,passedLookbehind:n,strategy:r}){if(t==="strict"&&n&&r)throw new Error(m`Uses "\G" in a way that requires non-strict accuracy`)}},Subroutine({node:e},{jsGroupNameMap:t}){let{ref:n}=e;typeof n=="string"&&!Xe(n)&&(n=Ze(n,t),e.ref=n)}},cs={Backreference({node:e},{multiplexCapturesToLeftByRef:t,reffedNodesByReferencer:n}){let{orphan:r,ref:s}=e;r||n.set(e,[...t.get(s).map(({node:i})=>i)])},CapturingGroup:{enter({node:e,parent:t,replaceWith:n,skip:r},{groupOriginByCopy:s,groupsByName:i,multiplexCapturesToLeftByRef:a,openRefs:o,reffedNodesByReferencer:c}){let u=s.get(e);if(u&&o.has(e.number)){let p=j(Jt(e.number),t);c.set(p,o.get(e.number)),n(p);return}o.set(e.number,e),a.set(e.number,[]),e.name&&ce(a,e.name,[]);let l=a.get(e.name??e.number);for(let p=0;p<l.length;p++){let h=l[p];if(u===h.node||u&&u===h.origin||e===h.origin){l.splice(p,1);break}}if(a.get(e.number).push({node:e,origin:u}),e.name&&a.get(e.name).push({node:e,origin:u}),e.name){let p=ce(i,e.name,new Map),h=!1;if(u)h=!0;else for(let d of p.values())if(!d.hasDuplicateNameToRemove){h=!0;break}i.get(e.name).set(e,{node:e,hasDuplicateNameToRemove:h})}},exit({node:e},{openRefs:t}){t.get(e.number)===e&&t.delete(e.number)}},Group:{enter({node:e},t){t.prevFlags=t.currentFlags,e.flags&&(t.currentFlags=xe(t.currentFlags,e.flags))},exit(e,t){t.currentFlags=t.prevFlags}},Subroutine({node:e,parent:t,replaceWith:n},r){let{isRecursive:s,ref:i}=e;if(s){let l=t;for(;(l=l.parent)&&!(l.type==="CapturingGroup"&&(l.name===i||l.number===i)););r.reffedNodesByReferencer.set(e,l);return}let a=r.subroutineRefMap.get(i),o=i===0,c=o?Jt(0):an(a,r.groupOriginByCopy,null),u=c;if(!o){let l=on(hs(a,h=>h.type==="Group"&&!!h.flags)),p=l?xe(r.globalFlags,l):r.globalFlags;ls(p,r.currentFlags)||(u=E({flags:ds(p)}),u.body[0].body.push(c))}n(S(u,t),{traverse:!o})}},us={Backreference({node:e,parent:t,replaceWith:n},r){if(e.orphan){r.highestOrphanBackref=Math.max(r.highestOrphanBackref,e.ref);return}let i=r.reffedNodesByReferencer.get(e).filter(a=>ps(a,e));if(!i.length)n(S(U({negate:!0}),t));else if(i.length>1){let a=E({atomic:!0,body:i.reverse().map(o=>T({body:[ye(o.number)]}))});n(S(a,t))}else e.ref=i[0].number},CapturingGroup({node:e},t){e.number=++t.numCapturesToLeft,e.name&&t.groupsByName.get(e.name).get(e).hasDuplicateNameToRemove&&delete e.name},Regex:{exit({node:e},t){let n=Math.max(t.highestOrphanBackref-t.numCapturesToLeft,0);for(let r=0;r<n;r++){let s=Be();e.body.at(-1).body.push(s)}}},Subroutine({node:e},t){!e.isRecursive||e.ref===0||(e.ref=t.reffedNodesByReferencer.get(e).number)}};function sn(e){V(e,{"*"({node:t,parent:n}){t.parent=n}})}function ls(e,t){return e.dotAll===t.dotAll&&e.ignoreCase===t.ignoreCase}function ps(e,t){let n=t;do{if(n.type==="Regex")return!1;if(n.type==="Alternative")continue;if(n===e)return!1;let r=cn(n.parent);for(let s of r){if(s===n)break;if(s===e||ln(s,e))return!0}}while(n=n.parent);throw new Error("Unexpected path")}function an(e,t,n,r){let s=Array.isArray(e)?[]:{};for(let[i,a]of Object.entries(e))i==="parent"?s.parent=Array.isArray(n)?r:n:a&&typeof a=="object"?s[i]=an(a,t,s,n):(i==="type"&&a==="CapturingGroup"&&t.set(s,t.get(e)??e),s[i]=a);return s}function Jt(e){let t=De(e);return t.isRecursive=!0,t}function hs(e,t){let n=[];for(;e=e.parent;)(!t||t(e))&&n.push(e);return n}function Ze(e,t){if(t.has(e))return t.get(e);let n=`$${t.size}_${e.replace(/^[^$_\p{IDS}]|[^$\u200C\u200D\p{IDC}]/ug,"_")}`;return t.set(e,n),n}function on(e){let t=["dotAll","ignoreCase"],n={enable:{},disable:{}};return e.forEach(({flags:r})=>{t.forEach(s=>{r.enable?.[s]&&(delete n.disable[s],n.enable[s]=!0),r.disable?.[s]&&(n.disable[s]=!0)})}),Object.keys(n.enable).length||delete n.enable,Object.keys(n.disable).length||delete n.disable,n.enable||n.disable?n:null}function ds({dotAll:e,ignoreCase:t}){let n={};return(e||t)&&(n.enable={},e&&(n.enable.dotAll=!0),t&&(n.enable.ignoreCase=!0)),(!e||!t)&&(n.disable={},!e&&(n.disable.dotAll=!0),!t&&(n.disable.ignoreCase=!0)),n}function cn(e){if(!e)throw new Error("Node expected");let{body:t}=e;return Array.isArray(t)?t:t?[t]:null}function un(e){let t=e.find(n=>n.kind==="search_start"||ms(n,{negate:!1})||!fs(n));if(!t)return null;if(t.kind==="search_start")return t;if(t.type==="LookaroundAssertion")return t.body[0].body[0];if(t.type==="CapturingGroup"||t.type==="Group"){let n=[];for(let r of t.body){let s=un(r.body);if(!s)return null;Array.isArray(s)?n.push(...s):n.push(s)}return n}return null}function ln(e,t){let n=cn(e)??[];for(let r of n)if(r===t||ln(r,t))return!0;return!1}function fs({type:e}){return e==="Assertion"||e==="Directive"||e==="LookaroundAssertion"}function gs(e){let t=["Character","CharacterClass","CharacterSet"];return t.includes(e.type)||e.type==="Quantifier"&&e.min&&t.includes(e.body.type)}function ms(e,t){let n={negate:null,...t};return e.type==="LookaroundAssertion"&&(n.negate===null||e.negate===n.negate)&&e.body.length===1&&be(e.body[0],{type:"Assertion",kind:"search_start"})}function Xe(e){return/^[$_\p{IDS}][$\u200C\u200D\p{IDC}]*$/u.test(e)}function B(e,t){let r=we(e,{...t,unicodePropertyMap:Ye}).body;return r.length>1||r[0].body.length>1?E({body:r}):r[0].body[0]}function Qe(e,t){return e.negate=t,e}function j(e,t){return e.parent=t,e}function S(e,t){return sn(e),e.parent=t,e}function bs(e,t){let n=tn(t),r=Je(n.target,"ES2024"),s=Je(n.target,"ES2025"),i=n.rules.recursionLimit;if(!Number.isInteger(i)||i<2||i>20)throw new Error("Invalid recursionLimit; use 2-20");let a=null,o=null;if(!s){let d=[e.flags.ignoreCase];V(e,ys,{getCurrentModI:()=>d.at(-1),popModI(){d.pop()},pushModI(f){d.push(f)},setHasCasedChar(){d.at(-1)?a=!0:o=!0}})}let c={dotAll:e.flags.dotAll,ignoreCase:!!((e.flags.ignoreCase||a)&&!o)},u=e,l={accuracy:n.accuracy,appliedGlobalFlags:c,captureMap:new Map,currentFlags:{dotAll:e.flags.dotAll,ignoreCase:e.flags.ignoreCase},inCharClass:!1,lastNode:u,originMap:e._originMap,recursionLimit:i,useAppliedIgnoreCase:!!(!s&&a&&o),useFlagMods:s,useFlagV:r,verbose:n.verbose};function p(d){return l.lastNode=u,u=d,Jr(ws[d.type],`Unexpected node type "${d.type}"`)(d,l,p)}let h={pattern:e.body.map(p).join("|"),flags:p(e.flags),options:{...e.options}};return r||(delete h.options.force.v,h.options.disable.v=!0,h.options.unicodeSetsPlugin=null),h._captureTransfers=new Map,h._hiddenCaptures=[],l.captureMap.forEach((d,f)=>{d.hidden&&h._hiddenCaptures.push(f),d.transferTo&&ce(h._captureTransfers,d.transferTo,[]).push(f)}),h}var ys={"*":{enter({node:e},t){if(Yt(e)){let n=t.getCurrentModI();t.pushModI(e.flags?xe({ignoreCase:n},e.flags).ignoreCase:n)}},exit({node:e},t){Yt(e)&&t.popModI()}},Backreference(e,t){t.setHasCasedChar()},Character({node:e},t){et(A(e.value))&&t.setHasCasedChar()},CharacterClassRange({node:e,skip:t},n){t(),pn(e,{firstOnly:!0}).length&&n.setHasCasedChar()},CharacterSet({node:e},t){e.kind==="property"&&rn.has(e.value)&&t.setHasCasedChar()}},ws={Alternative({body:e},t,n){return e.map(n).join("")},Assertion({kind:e,negate:t}){if(e==="string_end")return"$";if(e==="string_start")return"^";if(e==="word_boundary")return t?m`\B`:m`\b`;throw new Error(`Unexpected assertion kind "${e}"`)},Backreference({ref:e},t){if(typeof e!="number")throw new Error("Unexpected named backref in transformed AST");if(!t.useFlagMods&&t.accuracy==="strict"&&t.currentFlags.ignoreCase&&!t.captureMap.get(e).ignoreCase)throw new Error("Use of case-insensitive backref to case-sensitive group requires target ES2025 or non-strict accuracy");return"\\"+e},CapturingGroup(e,t,n){let{body:r,name:s,number:i}=e,a={ignoreCase:t.currentFlags.ignoreCase},o=t.originMap.get(e);return o&&(a.hidden=!0,i>o.number&&(a.transferTo=o.number)),t.captureMap.set(i,a),`(${s?`?<${s}>`:""}${r.map(n).join("|")})`},Character({value:e},t){let n=A(e),r=te(e,{escDigit:t.lastNode.type==="Backreference",inCharClass:t.inCharClass,useFlagV:t.useFlagV});if(r!==n)return r;if(t.useAppliedIgnoreCase&&t.currentFlags.ignoreCase&&et(n)){let s=nn(n);return t.inCharClass?s.join(""):s.length>1?`[${s.join("")}]`:s[0]}return n},CharacterClass(e,t,n){let{kind:r,negate:s,parent:i}=e,{body:a}=e;if(r==="intersection"&&!t.useFlagV)throw new Error("Use of character class intersection requires min target ES2024");P.bugFlagVLiteralHyphenIsRange&&t.useFlagV&&a.some(en)&&(a=[K(45),...a.filter(u=>!en(u))]);let o=()=>`[${s?"^":""}${a.map(n).join(r==="intersection"?"&&":"")}]`;if(!t.inCharClass){if((!t.useFlagV||P.bugNestedClassIgnoresNegation)&&!s){let l=a.filter(p=>p.type==="CharacterClass"&&p.kind==="union"&&p.negate);if(l.length){let p=E(),h=p.body[0];return p.parent=i,h.parent=p,a=a.filter(d=>!l.includes(d)),e.body=a,a.length?(e.parent=h,h.body.push(e)):p.body.pop(),l.forEach(d=>{let f=T({body:[d]});d.parent=f,f.parent=p,p.body.push(f)}),n(p)}}t.inCharClass=!0;let u=o();return t.inCharClass=!1,u}let c=a[0];if(r==="union"&&!s&&c&&((!t.useFlagV||!t.verbose)&&i.kind==="union"&&!(P.bugFlagVLiteralHyphenIsRange&&t.useFlagV)||!t.verbose&&i.kind==="intersection"&&a.length===1&&c.type!=="CharacterClassRange"))return a.map(n).join("");if(!t.useFlagV&&i.type==="CharacterClass")throw new Error("Uses nested character class in a way that requires min target ES2024");return o()},CharacterClassRange(e,t){let n=e.min.value,r=e.max.value,s={escDigit:!1,inCharClass:!0,useFlagV:t.useFlagV},i=te(n,s),a=te(r,s),o=new Set;if(t.useAppliedIgnoreCase&&t.currentFlags.ignoreCase){let c=pn(e);xs(c).forEach(l=>{o.add(Array.isArray(l)?`${te(l[0],s)}-${te(l[1],s)}`:te(l,s))})}return`${i}-${a}${[...o].join("")}`},CharacterSet({kind:e,negate:t,value:n,key:r},s){if(e==="dot")return s.currentFlags.dotAll?s.appliedGlobalFlags.dotAll||s.useFlagMods?".":"[^]":m`[^\n]`;if(e==="digit")return t?m`\D`:m`\d`;if(e==="property"){if(s.useAppliedIgnoreCase&&s.currentFlags.ignoreCase&&rn.has(n))throw new Error(`Unicode property "${n}" can't be case-insensitive when other chars have specific case`);return`${t?m`\P`:m`\p`}{${r?`${r}=`:""}${n}}`}if(e==="word")return t?m`\W`:m`\w`;throw new Error(`Unexpected character set kind "${e}"`)},Flags(e,t){return(t.appliedGlobalFlags.ignoreCase?"i":"")+(e.dotAll?"s":"")+(e.sticky?"y":"")},Group({atomic:e,body:t,flags:n,parent:r},s,i){let a=s.currentFlags;n&&(s.currentFlags=xe(a,n));let o=t.map(i).join("|"),c=!s.verbose&&t.length===1&&r.type!=="Quantifier"&&!e&&(!s.useFlagMods||!n)?o:`(?${As(e,n,s.useFlagMods)}${o})`;return s.currentFlags=a,c},LookaroundAssertion({body:e,kind:t,negate:n},r,s){return`(?${`${t==="lookahead"?"":"<"}${n?"!":"="}`}${e.map(s).join("|")})`},Quantifier(e,t,n){return n(e.body)+Is(e)},Subroutine({isRecursive:e,ref:t},n){if(!e)throw new Error("Unexpected non-recursive subroutine in transformed AST");let r=n.recursionLimit;return t===0?`(?R=${r})`:m`\g<${t}&R=${r}>`}},Cs=new Set(["$","(",")","*","+",".","?","[","\\","]","^","{","|","}"]),_s=new Set(["-","\\","]","^","["]),Ss=new Set(["(",")","-","/","[","\\","]","^","{","|","}","!","#","$","%","&","*","+",",",".",":",";","<","=",">","?","@","`","~"]),Kt=new Map([[9,m`\t`],[10,m`\n`],[11,m`\v`],[12,m`\f`],[13,m`\r`],[8232,m`\u2028`],[8233,m`\u2029`],[65279,m`\uFEFF`]]),ks=new RegExp("^\\p{Cased}$","u");function et(e){return ks.test(e)}function pn(e,t){let n=!!t?.firstOnly,r=e.min.value,s=e.max.value,i=[];if(r<65&&(s===65535||s>=131071)||r===65536&&s>=131071)return i;for(let a=r;a<=s;a++){let o=A(a);if(!et(o))continue;let c=nn(o).filter(u=>{let l=u.codePointAt(0);return l<r||l>s});if(c.length&&(i.push(...c),n))break}return i}function te(e,{escDigit:t,inCharClass:n,useFlagV:r}){if(Kt.has(e))return Kt.get(e);if(e<32||e>126&&e<160||e>262143||t&&Rs(e))return e>255?`\\u{${e.toString(16).toUpperCase()}}`:`\\x${e.toString(16).toUpperCase().padStart(2,"0")}`;let s=n?r?Ss:_s:Cs,i=A(e);return(s.has(i)?"\\":"")+i}function xs(e){let t=e.map(s=>s.codePointAt(0)).sort((s,i)=>s-i),n=[],r=null;for(let s=0;s<t.length;s++)t[s+1]===t[s]+1?r??=t[s]:r===null?n.push(t[s]):(n.push([r,t[s]]),r=null);return n}function As(e,t,n){if(e)return">";let r="";if(t&&n){let{enable:s,disable:i}=t;r=(s?.ignoreCase?"i":"")+(s?.dotAll?"s":"")+(i?"-":"")+(i?.ignoreCase?"i":"")+(i?.dotAll?"s":"")}return`${r}:`}function Is({kind:e,max:t,min:n}){let r;return!n&&t===1?r="?":!n&&t===1/0?r="*":n===1&&t===1/0?r="+":n===t?r=`{${n}}`:r=`{${n},${t===1/0?"":t}}`,r+{greedy:"",lazy:"?",possessive:"+"}[e]}function Yt({type:e}){return e==="CapturingGroup"||e==="Group"||e==="LookaroundAssertion"}function Rs(e){return e>47&&e<58}function en({type:e,value:t}){return e==="Character"&&t===45}var Es=class Ke extends RegExp{#t=new Map;#e=null;#r;#n=null;#s=null;rawOptions={};get source(){return this.#r||"(?:)"}constructor(t,n,r){let s=!!r?.lazyCompile;if(t instanceof RegExp){if(r)throw new Error("Cannot provide options when copying a regexp");let i=t;super(i,n),this.#r=i.source,i instanceof Ke&&(this.#t=i.#t,this.#n=i.#n,this.#s=i.#s,this.rawOptions=i.rawOptions)}else{let i={hiddenCaptures:[],strategy:null,transfers:[],...r};super(s?"":t,n),this.#r=t,this.#t=Ns(i.hiddenCaptures,i.transfers),this.#s=i.strategy,this.rawOptions=r??{}}s||(this.#e=this)}exec(t){if(!this.#e){let{lazyCompile:s,...i}=this.rawOptions;this.#e=new Ke(this.#r,this.flags,i)}let n=this.global||this.sticky,r=this.lastIndex;if(this.#s==="clip_search"&&n&&r){this.lastIndex=0;let s=this.#i(t.slice(r));return s&&(vs(s,r,t,this.hasIndices),this.lastIndex+=r),s}return this.#i(t)}#i(t){this.#e.lastIndex=this.lastIndex;let n=super.exec.call(this.#e,t);if(this.lastIndex=this.#e.lastIndex,!n||!this.#t.size)return n;let r=[...n];n.length=1;let s;this.hasIndices&&(s=[...n.indices],n.indices.length=1);let i=[0];for(let a=1;a<r.length;a++){let{hidden:o,transferTo:c}=this.#t.get(a)??{};if(o?i.push(null):(i.push(n.length),n.push(r[a]),this.hasIndices&&n.indices.push(s[a])),c&&r[a]!==void 0){let u=i[c];if(!u)throw new Error(`Invalid capture transfer to "${u}"`);if(n[u]=r[a],this.hasIndices&&(n.indices[u]=s[a]),n.groups){this.#n||(this.#n=Ps(this.source));let l=this.#n.get(c);l&&(n.groups[l]=r[a],this.hasIndices&&(n.indices.groups[l]=s[a]))}}}return n}};function vs(e,t,n,r){if(e.index+=t,e.input=n,r){let s=e.indices;for(let a=0;a<s.length;a++){let o=s[a];o&&(s[a]=[o[0]+t,o[1]+t])}let i=s.groups;i&&Object.keys(i).forEach(a=>{let o=i[a];o&&(i[a]=[o[0]+t,o[1]+t])})}}function Ns(e,t){let n=new Map;for(let r of e)n.set(r,{hidden:!0});for(let[r,s]of t)for(let i of s)ce(n,i,{}).transferTo=r;return n}function Ps(e){let t=/(?<capture>\((?:\?<(?![=!])(?<name>[^>]+)>|(?!\?)))|\\?./gsu,n=new Map,r=0,s=0,i;for(;i=t.exec(e);){let{0:a,groups:{capture:o,name:c}}=i;a==="["?r++:r?a==="]"&&r--:o&&(s++,c&&n.set(s,c))}return n}function hn(e,t){let n=$s(e,t);return n.options?new Es(n.pattern,n.flags,n.options):new RegExp(n.pattern,n.flags)}function $s(e,t){let n=tn(t),r=we(e,{flags:n.flags,normalizeUnknownPropertyNames:!0,rules:{captureGroup:n.rules.captureGroup,singleline:n.rules.singleline},skipBackrefValidation:n.rules.allowOrphanBackrefs,unicodePropertyMap:Ye}),s=as(r,{accuracy:n.accuracy,asciiWordBoundaries:n.rules.asciiWordBoundaries,avoidSubclass:n.avoidSubclass,bestEffortTarget:n.target}),i=bs(s,n),a=Xt(i.pattern,{captureTransfers:i._captureTransfers,hiddenCaptures:i._hiddenCaptures,mode:"external"}),o=ze(a.pattern),c=je(o.pattern,{captureTransfers:a.captureTransfers,hiddenCaptures:a.hiddenCaptures}),u={pattern:c.pattern,flags:`${n.hasIndices?"d":""}${n.global?"g":""}${i.flags}${i.options.disable.v?"u":"v"}`};if(n.avoidSubclass){if(n.lazyCompileLength!==1/0)throw new Error("Lazy compilation requires subclass")}else{let l=c.hiddenCaptures.sort((f,g)=>f-g),p=Array.from(c.captureTransfers),h=s._strategy,d=u.pattern.length>=n.lazyCompileLength;(l.length||p.length||h||d)&&(u.options={...l.length&&{hiddenCaptures:l},...p.length&&{transfers:p},...h&&{strategy:h},...d&&{lazyCompile:d}})}return u}function dn(e,t){return hn(e,{global:!0,hasIndices:!0,lazyCompileLength:3e3,rules:{allowOrphanBackrefs:!0,asciiWordBoundaries:!0,captureGroup:!0,recursionLimit:5,singleline:!0},...t})}function tt(e={}){let t={target:"auto",cache:new Map,...e};return t.regexConstructor||=n=>dn(n,{target:t.target}),{createScanner(n){return new wt(n,t)},createString(n){return{content:n}}}}function Ls(e){return ht(e)}function ht(e){return Array.isArray(e)?Gs(e):e instanceof RegExp?e:typeof e=="object"?Ms(e):e}function Gs(e){let t=[];for(let n=0,r=e.length;n<r;n++)t[n]=ht(e[n]);return t}function Ms(e){let t={};for(let n in e)t[n]=ht(e[n]);return t}function Sn(e,...t){return t.forEach(n=>{for(let r in n)e[r]=n[r]}),e}function kn(e){let t=~e.lastIndexOf("/")||~e.lastIndexOf("\\");return t===0?e:~t===e.length-1?kn(e.substring(0,e.length-1)):e.substr(~t+1)}var nt=/\$(\d+)|\${(\d+):\/(downcase|upcase)}/g,Ie=class{static hasCaptures(e){return e===null?!1:(nt.lastIndex=0,nt.test(e))}static replaceCaptures(e,t,n){return e.replace(nt,(r,s,i,a)=>{let o=n[parseInt(s||i,10)];if(o){let c=t.substring(o.start,o.end);for(;c[0]===".";)c=c.substring(1);switch(a){case"downcase":return c.toLowerCase();case"upcase":return c.toUpperCase();default:return c}}else return r})}};function xn(e,t){return e<t?-1:e>t?1:0}function An(e,t){if(e===null&&t===null)return 0;if(!e)return-1;if(!t)return 1;let n=e.length,r=t.length;if(n===r){for(let s=0;s<n;s++){let i=xn(e[s],t[s]);if(i!==0)return i}return 0}return n-r}function fn(e){return!!(/^#[0-9a-f]{6}$/i.test(e)||/^#[0-9a-f]{8}$/i.test(e)||/^#[0-9a-f]{3}$/i.test(e)||/^#[0-9a-f]{4}$/i.test(e))}function In(e){return e.replace(/[\-\\\{\}\*\+\?\|\^\$\.\,\[\]\(\)\#\s]/g,"\\$&")}var Rn=class{constructor(e){this.fn=e}cache=new Map;get(e){if(this.cache.has(e))return this.cache.get(e);let t=this.fn(e);return this.cache.set(e,t),t}},it=class{constructor(e,t,n){this._colorMap=e,this._defaults=t,this._root=n}static createFromRawTheme(e,t){return this.createFromParsedTheme(Bs(e),t)}static createFromParsedTheme(e,t){return Ds(e,t)}_cachedMatchRoot=new Rn(e=>this._root.match(e));getColorMap(){return this._colorMap.getColorMap()}getDefaults(){return this._defaults}match(e){if(e===null)return this._defaults;let t=e.scopeName,r=this._cachedMatchRoot.get(t).find(s=>Ts(e.parent,s.parentScopes));return r?new En(r.fontStyle,r.foreground,r.background):null}},rt=class Re{constructor(t,n){this.parent=t,this.scopeName=n}static push(t,n){for(let r of n)t=new Re(t,r);return t}static from(...t){let n=null;for(let r=0;r<t.length;r++)n=new Re(n,t[r]);return n}push(t){return new Re(this,t)}getSegments(){let t=this,n=[];for(;t;)n.push(t.scopeName),t=t.parent;return n.reverse(),n}toString(){return this.getSegments().join(" ")}extends(t){return this===t?!0:this.parent===null?!1:this.parent.extends(t)}getExtensionIfDefined(t){let n=[],r=this;for(;r&&r!==t;)n.push(r.scopeName),r=r.parent;return r===t?n.reverse():void 0}};function Ts(e,t){if(t.length===0)return!0;for(let n=0;n<t.length;n++){let r=t[n],s=!1;if(r===">"){if(n===t.length-1)return!1;r=t[++n],s=!0}for(;e&&!Os(e.scopeName,r);){if(s)return!1;e=e.parent}if(!e)return!1;e=e.parent}return!0}function Os(e,t){return t===e||e.startsWith(t)&&e[t.length]==="."}var En=class{constructor(e,t,n){this.fontStyle=e,this.foregroundId=t,this.backgroundId=n}};function Bs(e){if(!e)return[];if(!e.settings||!Array.isArray(e.settings))return[];let t=e.settings,n=[],r=0;for(let s=0,i=t.length;s<i;s++){let a=t[s];if(!a.settings)continue;let o;if(typeof a.scope=="string"){let p=a.scope;p=p.replace(/^[,]+/,""),p=p.replace(/[,]+$/,""),o=p.split(",")}else Array.isArray(a.scope)?o=a.scope:o=[""];let c=-1;if(typeof a.settings.fontStyle=="string"){c=0;let p=a.settings.fontStyle.split(" ");for(let h=0,d=p.length;h<d;h++)switch(p[h]){case"italic":c=c|1;break;case"bold":c=c|2;break;case"underline":c=c|4;break;case"strikethrough":c=c|8;break}}let u=null;typeof a.settings.foreground=="string"&&fn(a.settings.foreground)&&(u=a.settings.foreground);let l=null;typeof a.settings.background=="string"&&fn(a.settings.background)&&(l=a.settings.background);for(let p=0,h=o.length;p<h;p++){let f=o[p].trim().split(" "),g=f[f.length-1],b=null;f.length>1&&(b=f.slice(0,f.length-1),b.reverse()),n[r++]=new Fs(g,b,s,c,u,l)}}return n}var Fs=class{constructor(e,t,n,r,s,i){this.scope=e,this.parentScopes=t,this.index=n,this.fontStyle=r,this.foreground=s,this.background=i}};function Ds(e,t){e.sort((c,u)=>{let l=xn(c.scope,u.scope);return l!==0||(l=An(c.parentScopes,u.parentScopes),l!==0)?l:c.index-u.index});let n=0,r="#000000",s="#ffffff";for(;e.length>=1&&e[0].scope==="";){let c=e.shift();c.fontStyle!==-1&&(n=c.fontStyle),c.foreground!==null&&(r=c.foreground),c.background!==null&&(s=c.background)}let i=new Us(t),a=new En(n,i.getId(r),i.getId(s)),o=new js(new at(0,null,-1,0,0),[]);for(let c=0,u=e.length;c<u;c++){let l=e[c];o.insert(0,l.scope,l.parentScopes,l.fontStyle,i.getId(l.foreground),i.getId(l.background))}return new it(i,a,o)}var Us=class{_isFrozen;_lastColorId;_id2color;_color2id;constructor(e){if(this._lastColorId=0,this._id2color=[],this._color2id=Object.create(null),Array.isArray(e)){this._isFrozen=!0;for(let t=0,n=e.length;t<n;t++)this._color2id[e[t]]=t,this._id2color[t]=e[t]}else this._isFrozen=!1}getId(e){if(e===null)return 0;e=e.toUpperCase();let t=this._color2id[e];if(t)return t;if(this._isFrozen)throw new Error(`Missing color in color map - ${e}`);return t=++this._lastColorId,this._color2id[e]=t,this._id2color[t]=e,t}getColorMap(){return this._id2color.slice(0)}},Ws=Object.freeze([]),at=class vn{scopeDepth;parentScopes;fontStyle;foreground;background;constructor(t,n,r,s,i){this.scopeDepth=t,this.parentScopes=n||Ws,this.fontStyle=r,this.foreground=s,this.background=i}clone(){return new vn(this.scopeDepth,this.parentScopes,this.fontStyle,this.foreground,this.background)}static cloneArr(t){let n=[];for(let r=0,s=t.length;r<s;r++)n[r]=t[r].clone();return n}acceptOverwrite(t,n,r,s){this.scopeDepth>t?console.log("how did this happen?"):this.scopeDepth=t,n!==-1&&(this.fontStyle=n),r!==0&&(this.foreground=r),s!==0&&(this.background=s)}},js=class ot{constructor(t,n=[],r={}){this._mainRule=t,this._children=r,this._rulesWithParentScopes=n}_rulesWithParentScopes;static _cmpBySpecificity(t,n){if(t.scopeDepth!==n.scopeDepth)return n.scopeDepth-t.scopeDepth;let r=0,s=0;for(;t.parentScopes[r]===">"&&r++,n.parentScopes[s]===">"&&s++,!(r>=t.parentScopes.length||s>=n.parentScopes.length);){let i=n.parentScopes[s].length-t.parentScopes[r].length;if(i!==0)return i;r++,s++}return n.parentScopes.length-t.parentScopes.length}match(t){if(t!==""){let r=t.indexOf("."),s,i;if(r===-1?(s=t,i=""):(s=t.substring(0,r),i=t.substring(r+1)),this._children.hasOwnProperty(s))return this._children[s].match(i)}let n=this._rulesWithParentScopes.concat(this._mainRule);return n.sort(ot._cmpBySpecificity),n}insert(t,n,r,s,i,a){if(n===""){this._doInsertHere(t,r,s,i,a);return}let o=n.indexOf("."),c,u;o===-1?(c=n,u=""):(c=n.substring(0,o),u=n.substring(o+1));let l;this._children.hasOwnProperty(c)?l=this._children[c]:(l=new ot(this._mainRule.clone(),at.cloneArr(this._rulesWithParentScopes)),this._children[c]=l),l.insert(t+1,u,r,s,i,a)}_doInsertHere(t,n,r,s,i){if(n===null){this._mainRule.acceptOverwrite(t,r,s,i);return}for(let a=0,o=this._rulesWithParentScopes.length;a<o;a++){let c=this._rulesWithParentScopes[a];if(An(c.parentScopes,n)===0){c.acceptOverwrite(t,r,s,i);return}}r===-1&&(r=this._mainRule.fontStyle),s===0&&(s=this._mainRule.foreground),i===0&&(i=this._mainRule.background),this._rulesWithParentScopes.push(new at(t,n,r,s,i))}},ve=class N{static toBinaryStr(t){return t.toString(2).padStart(32,"0")}static print(t){let n=N.getLanguageId(t),r=N.getTokenType(t),s=N.getFontStyle(t),i=N.getForeground(t),a=N.getBackground(t);console.log({languageId:n,tokenType:r,fontStyle:s,foreground:i,background:a})}static getLanguageId(t){return(t&255)>>>0}static getTokenType(t){return(t&768)>>>8}static containsBalancedBrackets(t){return(t&1024)!==0}static getFontStyle(t){return(t&30720)>>>11}static getForeground(t){return(t&16744448)>>>15}static getBackground(t){return(t&4278190080)>>>24}static set(t,n,r,s,i,a,o){let c=N.getLanguageId(t),u=N.getTokenType(t),l=N.containsBalancedBrackets(t)?1:0,p=N.getFontStyle(t),h=N.getForeground(t),d=N.getBackground(t);return n!==0&&(c=n),r!==8&&(u=r),s!==null&&(l=s?1:0),i!==-1&&(p=i),a!==0&&(h=a),o!==0&&(d=o),(c<<0|u<<8|l<<10|p<<11|h<<15|d<<24)>>>0}};function Ne(e,t){let n=[],r=zs(e),s=r.next();for(;s!==null;){let c=0;if(s.length===2&&s.charAt(1)===":"){switch(s.charAt(0)){case"R":c=1;break;case"L":c=-1;break;default:console.log(`Unknown priority ${s} in scope selector`)}s=r.next()}let u=a();if(n.push({matcher:u,priority:c}),s!==",")break;s=r.next()}return n;function i(){if(s==="-"){s=r.next();let c=i();return u=>!!c&&!c(u)}if(s==="("){s=r.next();let c=o();return s===")"&&(s=r.next()),c}if(gn(s)){let c=[];do c.push(s),s=r.next();while(gn(s));return u=>t(c,u)}return null}function a(){let c=[],u=i();for(;u;)c.push(u),u=i();return l=>c.every(p=>p(l))}function o(){let c=[],u=a();for(;u&&(c.push(u),s==="|"||s===",");){do s=r.next();while(s==="|"||s===",");u=a()}return l=>c.some(p=>p(l))}}function gn(e){return!!e&&!!e.match(/[\w\.:]+/)}function zs(e){let t=/([LR]:|[\w\.:][\w\.:\-]*|[\,\|\-\(\)])/g,n=t.exec(e);return{next:()=>{if(!n)return null;let r=n[0];return n=t.exec(e),r}}}function Nn(e){typeof e.dispose=="function"&&e.dispose()}var pe=class{constructor(e){this.scopeName=e}toKey(){return this.scopeName}},qs=class{constructor(e,t){this.scopeName=e,this.ruleName=t}toKey(){return`${this.scopeName}#${this.ruleName}`}},Hs=class{_references=[];_seenReferenceKeys=new Set;get references(){return this._references}visitedRule=new Set;add(e){let t=e.toKey();this._seenReferenceKeys.has(t)||(this._seenReferenceKeys.add(t),this._references.push(e))}},Vs=class{constructor(e,t){this.repo=e,this.initialScopeName=t,this.seenFullScopeRequests.add(this.initialScopeName),this.Q=[new pe(this.initialScopeName)]}seenFullScopeRequests=new Set;seenPartialScopeRequests=new Set;Q;processQueue(){let e=this.Q;this.Q=[];let t=new Hs;for(let n of e)Zs(n,this.initialScopeName,this.repo,t);for(let n of t.references)if(n instanceof pe){if(this.seenFullScopeRequests.has(n.scopeName))continue;this.seenFullScopeRequests.add(n.scopeName),this.Q.push(n)}else{if(this.seenFullScopeRequests.has(n.scopeName)||this.seenPartialScopeRequests.has(n.toKey()))continue;this.seenPartialScopeRequests.add(n.toKey()),this.Q.push(n)}}};function Zs(e,t,n,r){let s=n.lookup(e.scopeName);if(!s){if(e.scopeName===t)throw new Error(`No grammar provided for <${t}>`);return}let i=n.lookup(t);e instanceof pe?Ee({baseGrammar:i,selfGrammar:s},r):ct(e.ruleName,{baseGrammar:i,selfGrammar:s,repository:s.repository},r);let a=n.injections(e.scopeName);if(a)for(let o of a)r.add(new pe(o))}function ct(e,t,n){if(t.repository&&t.repository[e]){let r=t.repository[e];Pe([r],t,n)}}function Ee(e,t){e.selfGrammar.patterns&&Array.isArray(e.selfGrammar.patterns)&&Pe(e.selfGrammar.patterns,{...e,repository:e.selfGrammar.repository},t),e.selfGrammar.injections&&Pe(Object.values(e.selfGrammar.injections),{...e,repository:e.selfGrammar.repository},t)}function Pe(e,t,n){for(let r of e){if(n.visitedRule.has(r))continue;n.visitedRule.add(r);let s=r.repository?Sn({},t.repository,r.repository):t.repository;Array.isArray(r.patterns)&&Pe(r.patterns,{...t,repository:s},n);let i=r.include;if(!i)continue;let a=Pn(i);switch(a.kind){case 0:Ee({...t,selfGrammar:t.baseGrammar},n);break;case 1:Ee(t,n);break;case 2:ct(a.ruleName,{...t,repository:s},n);break;case 3:case 4:let o=a.scopeName===t.selfGrammar.scopeName?t.selfGrammar:a.scopeName===t.baseGrammar.scopeName?t.baseGrammar:void 0;if(o){let c={baseGrammar:t.baseGrammar,selfGrammar:o,repository:s};a.kind===4?ct(a.ruleName,c,n):Ee(c,n)}else a.kind===4?n.add(new qs(a.scopeName,a.ruleName)):n.add(new pe(a.scopeName));break}}}var Xs=class{kind=0},Qs=class{kind=1},Js=class{constructor(e){this.ruleName=e}kind=2},Ks=class{constructor(e){this.scopeName=e}kind=3},Ys=class{constructor(e,t){this.scopeName=e,this.ruleName=t}kind=4};function Pn(e){if(e==="$base")return new Xs;if(e==="$self")return new Qs;let t=e.indexOf("#");if(t===-1)return new Ks(e);if(t===0)return new Js(e.substring(1));{let n=e.substring(0,t),r=e.substring(t+1);return new Ys(n,r)}}var ei=/\\(\d+)/,mn=/\\(\d+)/g;var ti=-1,$n=-2;var fe=class{$location;id;_nameIsCapturing;_name;_contentNameIsCapturing;_contentName;constructor(e,t,n,r){this.$location=e,this.id=t,this._name=n||null,this._nameIsCapturing=Ie.hasCaptures(this._name),this._contentName=r||null,this._contentNameIsCapturing=Ie.hasCaptures(this._contentName)}get debugName(){let e=this.$location?`${kn(this.$location.filename)}:${this.$location.line}`:"unknown";return`${this.constructor.name}#${this.id} @ ${e}`}getName(e,t){return!this._nameIsCapturing||this._name===null||e===null||t===null?this._name:Ie.replaceCaptures(this._name,e,t)}getContentName(e,t){return!this._contentNameIsCapturing||this._contentName===null?this._contentName:Ie.replaceCaptures(this._contentName,e,t)}},ni=class extends fe{retokenizeCapturedWithRuleId;constructor(e,t,n,r,s){super(e,t,n,r),this.retokenizeCapturedWithRuleId=s}dispose(){}collectPatterns(e,t){throw new Error("Not supported!")}compile(e,t){throw new Error("Not supported!")}compileAG(e,t,n,r){throw new Error("Not supported!")}},ri=class extends fe{_match;captures;_cachedCompiledPatterns;constructor(e,t,n,r,s){super(e,t,n,null),this._match=new he(r,this.id),this.captures=s,this._cachedCompiledPatterns=null}dispose(){this._cachedCompiledPatterns&&(this._cachedCompiledPatterns.dispose(),this._cachedCompiledPatterns=null)}get debugMatchRegExp(){return`${this._match.source}`}collectPatterns(e,t){t.push(this._match)}compile(e,t){return this._getCachedCompiledPatterns(e).compile(e)}compileAG(e,t,n,r){return this._getCachedCompiledPatterns(e).compileAG(e,n,r)}_getCachedCompiledPatterns(e){return this._cachedCompiledPatterns||(this._cachedCompiledPatterns=new de,this.collectPatterns(e,this._cachedCompiledPatterns)),this._cachedCompiledPatterns}},bn=class extends fe{hasMissingPatterns;patterns;_cachedCompiledPatterns;constructor(e,t,n,r,s){super(e,t,n,r),this.patterns=s.patterns,this.hasMissingPatterns=s.hasMissingPatterns,this._cachedCompiledPatterns=null}dispose(){this._cachedCompiledPatterns&&(this._cachedCompiledPatterns.dispose(),this._cachedCompiledPatterns=null)}collectPatterns(e,t){for(let n of this.patterns)e.getRule(n).collectPatterns(e,t)}compile(e,t){return this._getCachedCompiledPatterns(e).compile(e)}compileAG(e,t,n,r){return this._getCachedCompiledPatterns(e).compileAG(e,n,r)}_getCachedCompiledPatterns(e){return this._cachedCompiledPatterns||(this._cachedCompiledPatterns=new de,this.collectPatterns(e,this._cachedCompiledPatterns)),this._cachedCompiledPatterns}},ut=class extends fe{_begin;beginCaptures;_end;endHasBackReferences;endCaptures;applyEndPatternLast;hasMissingPatterns;patterns;_cachedCompiledPatterns;constructor(e,t,n,r,s,i,a,o,c,u){super(e,t,n,r),this._begin=new he(s,this.id),this.beginCaptures=i,this._end=new he(a||"￿",-1),this.endHasBackReferences=this._end.hasBackReferences,this.endCaptures=o,this.applyEndPatternLast=c||!1,this.patterns=u.patterns,this.hasMissingPatterns=u.hasMissingPatterns,this._cachedCompiledPatterns=null}dispose(){this._cachedCompiledPatterns&&(this._cachedCompiledPatterns.dispose(),this._cachedCompiledPatterns=null)}get debugBeginRegExp(){return`${this._begin.source}`}get debugEndRegExp(){return`${this._end.source}`}getEndWithResolvedBackReferences(e,t){return this._end.resolveBackReferences(e,t)}collectPatterns(e,t){t.push(this._begin)}compile(e,t){return this._getCachedCompiledPatterns(e,t).compile(e)}compileAG(e,t,n,r){return this._getCachedCompiledPatterns(e,t).compileAG(e,n,r)}_getCachedCompiledPatterns(e,t){if(!this._cachedCompiledPatterns){this._cachedCompiledPatterns=new de;for(let n of this.patterns)e.getRule(n).collectPatterns(e,this._cachedCompiledPatterns);this.applyEndPatternLast?this._cachedCompiledPatterns.push(this._end.hasBackReferences?this._end.clone():this._end):this._cachedCompiledPatterns.unshift(this._end.hasBackReferences?this._end.clone():this._end)}return this._end.hasBackReferences&&(this.applyEndPatternLast?this._cachedCompiledPatterns.setSource(this._cachedCompiledPatterns.length()-1,t):this._cachedCompiledPatterns.setSource(0,t)),this._cachedCompiledPatterns}},$e=class extends fe{_begin;beginCaptures;whileCaptures;_while;whileHasBackReferences;hasMissingPatterns;patterns;_cachedCompiledPatterns;_cachedCompiledWhilePatterns;constructor(e,t,n,r,s,i,a,o,c){super(e,t,n,r),this._begin=new he(s,this.id),this.beginCaptures=i,this.whileCaptures=o,this._while=new he(a,$n),this.whileHasBackReferences=this._while.hasBackReferences,this.patterns=c.patterns,this.hasMissingPatterns=c.hasMissingPatterns,this._cachedCompiledPatterns=null,this._cachedCompiledWhilePatterns=null}dispose(){this._cachedCompiledPatterns&&(this._cachedCompiledPatterns.dispose(),this._cachedCompiledPatterns=null),this._cachedCompiledWhilePatterns&&(this._cachedCompiledWhilePatterns.dispose(),this._cachedCompiledWhilePatterns=null)}get debugBeginRegExp(){return`${this._begin.source}`}get debugWhileRegExp(){return`${this._while.source}`}getWhileWithResolvedBackReferences(e,t){return this._while.resolveBackReferences(e,t)}collectPatterns(e,t){t.push(this._begin)}compile(e,t){return this._getCachedCompiledPatterns(e).compile(e)}compileAG(e,t,n,r){return this._getCachedCompiledPatterns(e).compileAG(e,n,r)}_getCachedCompiledPatterns(e){if(!this._cachedCompiledPatterns){this._cachedCompiledPatterns=new de;for(let t of this.patterns)e.getRule(t).collectPatterns(e,this._cachedCompiledPatterns)}return this._cachedCompiledPatterns}compileWhile(e,t){return this._getCachedCompiledWhilePatterns(e,t).compile(e)}compileWhileAG(e,t,n,r){return this._getCachedCompiledWhilePatterns(e,t).compileAG(e,n,r)}_getCachedCompiledWhilePatterns(e,t){return this._cachedCompiledWhilePatterns||(this._cachedCompiledWhilePatterns=new de,this._cachedCompiledWhilePatterns.push(this._while.hasBackReferences?this._while.clone():this._while)),this._while.hasBackReferences&&this._cachedCompiledWhilePatterns.setSource(0,t||"￿"),this._cachedCompiledWhilePatterns}},Ln=class I{static createCaptureRule(t,n,r,s,i){return t.registerRule(a=>new ni(n,a,r,s,i))}static getCompiledRuleId(t,n,r){return t.id||n.registerRule(s=>{if(t.id=s,t.match)return new ri(t.$vscodeTextmateLocation,t.id,t.name,t.match,I._compileCaptures(t.captures,n,r));if(typeof t.begin>"u"){t.repository&&(r=Sn({},r,t.repository));let i=t.patterns;return typeof i>"u"&&t.include&&(i=[{include:t.include}]),new bn(t.$vscodeTextmateLocation,t.id,t.name,t.contentName,I._compilePatterns(i,n,r))}return t.while?new $e(t.$vscodeTextmateLocation,t.id,t.name,t.contentName,t.begin,I._compileCaptures(t.beginCaptures||t.captures,n,r),t.while,I._compileCaptures(t.whileCaptures||t.captures,n,r),I._compilePatterns(t.patterns,n,r)):new ut(t.$vscodeTextmateLocation,t.id,t.name,t.contentName,t.begin,I._compileCaptures(t.beginCaptures||t.captures,n,r),t.end,I._compileCaptures(t.endCaptures||t.captures,n,r),t.applyEndPatternLast,I._compilePatterns(t.patterns,n,r))}),t.id}static _compileCaptures(t,n,r){let s=[];if(t){let i=0;for(let a in t){if(a==="$vscodeTextmateLocation")continue;let o=parseInt(a,10);o>i&&(i=o)}for(let a=0;a<=i;a++)s[a]=null;for(let a in t){if(a==="$vscodeTextmateLocation")continue;let o=parseInt(a,10),c=0;t[a].patterns&&(c=I.getCompiledRuleId(t[a],n,r)),s[o]=I.createCaptureRule(n,t[a].$vscodeTextmateLocation,t[a].name,t[a].contentName,c)}}return s}static _compilePatterns(t,n,r){let s=[];if(t)for(let i=0,a=t.length;i<a;i++){let o=t[i],c=-1;if(o.include){let u=Pn(o.include);switch(u.kind){case 0:case 1:c=I.getCompiledRuleId(r[o.include],n,r);break;case 2:let l=r[u.ruleName];l&&(c=I.getCompiledRuleId(l,n,r));break;case 3:case 4:let p=u.scopeName,h=u.kind===4?u.ruleName:null,d=n.getExternalGrammar(p,r);if(d)if(h){let f=d.repository[h];f&&(c=I.getCompiledRuleId(f,n,d.repository))}else c=I.getCompiledRuleId(d.repository.$self,n,d.repository);break}}else c=I.getCompiledRuleId(o,n,r);if(c!==-1){let u=n.getRule(c),l=!1;if((u instanceof bn||u instanceof ut||u instanceof $e)&&u.hasMissingPatterns&&u.patterns.length===0&&(l=!0),l)continue;s.push(c)}}return{patterns:s,hasMissingPatterns:(t?t.length:0)!==s.length}}},he=class Gn{source;ruleId;hasAnchor;hasBackReferences;_anchorCache;constructor(t,n){if(t&&typeof t=="string"){let r=t.length,s=0,i=[],a=!1;for(let o=0;o<r;o++)if(t.charAt(o)==="\\"&&o+1<r){let u=t.charAt(o+1);u==="z"?(i.push(t.substring(s,o)),i.push("$(?!\\n)(?<!\\n)"),s=o+2):(u==="A"||u==="G")&&(a=!0),o++}this.hasAnchor=a,s===0?this.source=t:(i.push(t.substring(s,r)),this.source=i.join(""))}else this.hasAnchor=!1,this.source=t;this.hasAnchor?this._anchorCache=this._buildAnchorCache():this._anchorCache=null,this.ruleId=n,typeof this.source=="string"?this.hasBackReferences=ei.test(this.source):this.hasBackReferences=!1}clone(){return new Gn(this.source,this.ruleId)}setSource(t){this.source!==t&&(this.source=t,this.hasAnchor&&(this._anchorCache=this._buildAnchorCache()))}resolveBackReferences(t,n){if(typeof this.source!="string")throw new Error("This method should only be called if the source is a string");let r=n.map(s=>t.substring(s.start,s.end));return mn.lastIndex=0,this.source.replace(mn,(s,i)=>In(r[parseInt(i,10)]||""))}_buildAnchorCache(){if(typeof this.source!="string")throw new Error("This method should only be called if the source is a string");let t=[],n=[],r=[],s=[],i,a,o,c;for(i=0,a=this.source.length;i<a;i++)o=this.source.charAt(i),t[i]=o,n[i]=o,r[i]=o,s[i]=o,o==="\\"&&i+1<a&&(c=this.source.charAt(i+1),c==="A"?(t[i+1]="￿",n[i+1]="￿",r[i+1]="A",s[i+1]="A"):c==="G"?(t[i+1]="￿",n[i+1]="G",r[i+1]="￿",s[i+1]="G"):(t[i+1]=c,n[i+1]=c,r[i+1]=c,s[i+1]=c),i++);return{A0_G0:t.join(""),A0_G1:n.join(""),A1_G0:r.join(""),A1_G1:s.join("")}}resolveAnchors(t,n){return!this.hasAnchor||!this._anchorCache||typeof this.source!="string"?this.source:t?n?this._anchorCache.A1_G1:this._anchorCache.A1_G0:n?this._anchorCache.A0_G1:this._anchorCache.A0_G0}},de=class{_items;_hasAnchors;_cached;_anchorCache;constructor(){this._items=[],this._hasAnchors=!1,this._cached=null,this._anchorCache={A0_G0:null,A0_G1:null,A1_G0:null,A1_G1:null}}dispose(){this._disposeCaches()}_disposeCaches(){this._cached&&(this._cached.dispose(),this._cached=null),this._anchorCache.A0_G0&&(this._anchorCache.A0_G0.dispose(),this._anchorCache.A0_G0=null),this._anchorCache.A0_G1&&(this._anchorCache.A0_G1.dispose(),this._anchorCache.A0_G1=null),this._anchorCache.A1_G0&&(this._anchorCache.A1_G0.dispose(),this._anchorCache.A1_G0=null),this._anchorCache.A1_G1&&(this._anchorCache.A1_G1.dispose(),this._anchorCache.A1_G1=null)}push(e){this._items.push(e),this._hasAnchors=this._hasAnchors||e.hasAnchor}unshift(e){this._items.unshift(e),this._hasAnchors=this._hasAnchors||e.hasAnchor}length(){return this._items.length}setSource(e,t){this._items[e].source!==t&&(this._disposeCaches(),this._items[e].setSource(t))}compile(e){if(!this._cached){let t=this._items.map(n=>n.source);this._cached=new yn(e,t,this._items.map(n=>n.ruleId))}return this._cached}compileAG(e,t,n){return this._hasAnchors?t?n?(this._anchorCache.A1_G1||(this._anchorCache.A1_G1=this._resolveAnchors(e,t,n)),this._anchorCache.A1_G1):(this._anchorCache.A1_G0||(this._anchorCache.A1_G0=this._resolveAnchors(e,t,n)),this._anchorCache.A1_G0):n?(this._anchorCache.A0_G1||(this._anchorCache.A0_G1=this._resolveAnchors(e,t,n)),this._anchorCache.A0_G1):(this._anchorCache.A0_G0||(this._anchorCache.A0_G0=this._resolveAnchors(e,t,n)),this._anchorCache.A0_G0):this.compile(e)}_resolveAnchors(e,t,n){let r=this._items.map(s=>s.resolveAnchors(t,n));return new yn(e,r,this._items.map(s=>s.ruleId))}},yn=class{constructor(e,t,n){this.regExps=t,this.rules=n,this.scanner=e.createOnigScanner(t)}scanner;dispose(){typeof this.scanner.dispose=="function"&&this.scanner.dispose()}toString(){let e=[];for(let t=0,n=this.rules.length;t<n;t++)e.push("   - "+this.rules[t]+": "+this.regExps[t]);return e.join(`
`)}findNextMatchSync(e,t,n){let r=this.scanner.findNextMatchSync(e,t,n);return r?{ruleId:this.rules[r.index],captureIndices:r.captureIndices}:null}},st=class{constructor(e,t){this.languageId=e,this.tokenType=t}},si=class lt{_defaultAttributes;_embeddedLanguagesMatcher;constructor(t,n){this._defaultAttributes=new st(t,8),this._embeddedLanguagesMatcher=new ii(Object.entries(n||{}))}getDefaultAttributes(){return this._defaultAttributes}getBasicScopeAttributes(t){return t===null?lt._NULL_SCOPE_METADATA:this._getBasicScopeAttributes.get(t)}static _NULL_SCOPE_METADATA=new st(0,0);_getBasicScopeAttributes=new Rn(t=>{let n=this._scopeToLanguage(t),r=this._toStandardTokenType(t);return new st(n,r)});_scopeToLanguage(t){return this._embeddedLanguagesMatcher.match(t)||0}_toStandardTokenType(t){let n=t.match(lt.STANDARD_TOKEN_TYPE_REGEXP);if(!n)return 8;switch(n[1]){case"comment":return 1;case"string":return 2;case"regex":return 3;case"meta.embedded":return 0}throw new Error("Unexpected match for standard token type!")}static STANDARD_TOKEN_TYPE_REGEXP=/\b(comment|string|regex|meta\.embedded)\b/},ii=class{values;scopesRegExp;constructor(e){if(e.length===0)this.values=null,this.scopesRegExp=null;else{this.values=new Map(e);let t=e.map(([n,r])=>In(n));t.sort(),t.reverse(),this.scopesRegExp=new RegExp(`^((${t.join(")|(")}))($|\\.)`,"")}}match(e){if(!this.scopesRegExp)return;let t=e.match(this.scopesRegExp);if(t)return this.values.get(t[1])}},la={InDebugMode:typeof process<"u"&&!!process.env.VSCODE_TEXTMATE_DEBUG},Mn=!1,wn=class{constructor(e,t){this.stack=e,this.stoppedEarly=t}};function Tn(e,t,n,r,s,i,a,o){let c=t.content.length,u=!1,l=-1;if(a){let d=ai(e,t,n,r,s,i);s=d.stack,r=d.linePos,n=d.isFirstLine,l=d.anchorPosition}let p=Date.now();for(;!u;){if(o!==0&&Date.now()-p>o)return new wn(s,!0);h()}return new wn(s,!1);function h(){let d=oi(e,t,n,r,s,l);if(!d){i.produce(s,c),u=!0;return}let f=d.captureIndices,g=d.matchedRuleId,b=f&&f.length>0?f[0].end>r:!1;if(g===ti){let y=s.getRule(e);i.produce(s,f[0].start),s=s.withContentNameScopesList(s.nameScopesList),ue(e,t,n,s,i,y.endCaptures,f),i.produce(s,f[0].end);let w=s;if(s=s.parent,l=w.getAnchorPos(),!b&&w.getEnterPos()===r){s=w,i.produce(s,c),u=!0;return}}else{let y=e.getRule(g);i.produce(s,f[0].start);let w=s,k=y.getName(t.content,f),C=s.contentNameScopesList.pushAttributed(k,e);if(s=s.push(g,r,l,f[0].end===c,null,C,C),y instanceof ut){let _=y;ue(e,t,n,s,i,_.beginCaptures,f),i.produce(s,f[0].end),l=f[0].end;let F=_.getContentName(t.content,f),$=C.pushAttributed(F,e);if(s=s.withContentNameScopesList($),_.endHasBackReferences&&(s=s.withEndRule(_.getEndWithResolvedBackReferences(t.content,f))),!b&&w.hasSameRuleAs(s)){s=s.pop(),i.produce(s,c),u=!0;return}}else if(y instanceof $e){let _=y;ue(e,t,n,s,i,_.beginCaptures,f),i.produce(s,f[0].end),l=f[0].end;let F=_.getContentName(t.content,f),$=C.pushAttributed(F,e);if(s=s.withContentNameScopesList($),_.whileHasBackReferences&&(s=s.withEndRule(_.getWhileWithResolvedBackReferences(t.content,f))),!b&&w.hasSameRuleAs(s)){s=s.pop(),i.produce(s,c),u=!0;return}}else if(ue(e,t,n,s,i,y.captures,f),i.produce(s,f[0].end),s=s.pop(),!b){s=s.safePop(),i.produce(s,c),u=!0;return}}f[0].end>r&&(r=f[0].end,n=!1)}}function ai(e,t,n,r,s,i){let a=s.beginRuleCapturedEOL?0:-1,o=[];for(let c=s;c;c=c.pop()){let u=c.getRule(e);u instanceof $e&&o.push({rule:u,stack:c})}for(let c=o.pop();c;c=o.pop()){let{ruleScanner:u,findOptions:l}=li(c.rule,e,c.stack.endRule,n,r===a),p=u.findNextMatchSync(t,r,l);if(p){if(p.ruleId!==$n){s=c.stack.pop();break}p.captureIndices&&p.captureIndices.length&&(i.produce(c.stack,p.captureIndices[0].start),ue(e,t,n,c.stack,i,c.rule.whileCaptures,p.captureIndices),i.produce(c.stack,p.captureIndices[0].end),a=p.captureIndices[0].end,p.captureIndices[0].end>r&&(r=p.captureIndices[0].end,n=!1))}else{s=c.stack.pop();break}}return{stack:s,linePos:r,anchorPosition:a,isFirstLine:n}}function oi(e,t,n,r,s,i){let a=ci(e,t,n,r,s,i),o=e.getInjections();if(o.length===0)return a;let c=ui(o,e,t,n,r,s,i);if(!c)return a;if(!a)return c;let u=a.captureIndices[0].start,l=c.captureIndices[0].start;return l<u||c.priorityMatch&&l===u?c:a}function ci(e,t,n,r,s,i){let a=s.getRule(e),{ruleScanner:o,findOptions:c}=On(a,e,s.endRule,n,r===i),u=o.findNextMatchSync(t,r,c);return u?{captureIndices:u.captureIndices,matchedRuleId:u.ruleId}:null}function ui(e,t,n,r,s,i,a){let o=Number.MAX_VALUE,c=null,u,l=0,p=i.contentNameScopesList.getScopeNames();for(let h=0,d=e.length;h<d;h++){let f=e[h];if(!f.matcher(p))continue;let g=t.getRule(f.ruleId),{ruleScanner:b,findOptions:y}=On(g,t,null,r,s===a),w=b.findNextMatchSync(n,s,y);if(!w)continue;let k=w.captureIndices[0].start;if(!(k>=o)&&(o=k,c=w.captureIndices,u=w.ruleId,l=f.priority,o===s))break}return c?{priorityMatch:l===-1,captureIndices:c,matchedRuleId:u}:null}function On(e,t,n,r,s){if(Mn){let a=e.compile(t,n),o=Bn(r,s);return{ruleScanner:a,findOptions:o}}return{ruleScanner:e.compileAG(t,n,r,s),findOptions:0}}function li(e,t,n,r,s){if(Mn){let a=e.compileWhile(t,n),o=Bn(r,s);return{ruleScanner:a,findOptions:o}}return{ruleScanner:e.compileWhileAG(t,n,r,s),findOptions:0}}function Bn(e,t){let n=0;return e||(n|=1),t||(n|=4),n}function ue(e,t,n,r,s,i,a){if(i.length===0)return;let o=t.content,c=Math.min(i.length,a.length),u=[],l=a[0].end;for(let p=0;p<c;p++){let h=i[p];if(h===null)continue;let d=a[p];if(d.length===0)continue;if(d.start>l)break;for(;u.length>0&&u[u.length-1].endPos<=d.start;)s.produceFromScopes(u[u.length-1].scopes,u[u.length-1].endPos),u.pop();if(u.length>0?s.produceFromScopes(u[u.length-1].scopes,d.start):s.produce(r,d.start),h.retokenizeCapturedWithRuleId){let g=h.getName(o,a),b=r.contentNameScopesList.pushAttributed(g,e),y=h.getContentName(o,a),w=b.pushAttributed(y,e),k=r.push(h.retokenizeCapturedWithRuleId,d.start,-1,!1,null,b,w),C=e.createOnigString(o.substring(0,d.end));Tn(e,C,n&&d.start===0,d.start,k,s,!1,0),Nn(C);continue}let f=h.getName(o,a);if(f!==null){let b=(u.length>0?u[u.length-1].scopes:r.contentNameScopesList).pushAttributed(f,e);u.push(new pi(b,d.end))}}for(;u.length>0;)s.produceFromScopes(u[u.length-1].scopes,u[u.length-1].endPos),u.pop()}var pi=class{scopes;endPos;constructor(e,t){this.scopes=e,this.endPos=t}};function hi(e,t,n,r,s,i,a,o){return new fi(e,t,n,r,s,i,a,o)}function Cn(e,t,n,r,s){let i=Ne(t,Le),a=Ln.getCompiledRuleId(n,r,s.repository);for(let o of i)e.push({debugSelector:t,matcher:o.matcher,ruleId:a,grammar:s,priority:o.priority})}function Le(e,t){if(t.length<e.length)return!1;let n=0;return e.every(r=>{for(let s=n;s<t.length;s++)if(di(t[s],r))return n=s+1,!0;return!1})}function di(e,t){if(!e)return!1;if(e===t)return!0;let n=t.length;return e.length>n&&e.substr(0,n)===t&&e[n]==="."}var fi=class{constructor(e,t,n,r,s,i,a,o){if(this._rootScopeName=e,this.balancedBracketSelectors=i,this._onigLib=o,this._basicScopeAttributesProvider=new si(n,r),this._rootId=-1,this._lastRuleId=0,this._ruleId2desc=[null],this._includedGrammars={},this._grammarRepository=a,this._grammar=_n(t,null),this._injections=null,this._tokenTypeMatchers=[],s)for(let c of Object.keys(s)){let u=Ne(c,Le);for(let l of u)this._tokenTypeMatchers.push({matcher:l.matcher,type:s[c]})}}_rootId;_lastRuleId;_ruleId2desc;_includedGrammars;_grammarRepository;_grammar;_injections;_basicScopeAttributesProvider;_tokenTypeMatchers;get themeProvider(){return this._grammarRepository}dispose(){for(let e of this._ruleId2desc)e&&e.dispose()}createOnigScanner(e){return this._onigLib.createOnigScanner(e)}createOnigString(e){return this._onigLib.createOnigString(e)}getMetadataForScope(e){return this._basicScopeAttributesProvider.getBasicScopeAttributes(e)}_collectInjections(){let e={lookup:s=>s===this._rootScopeName?this._grammar:this.getExternalGrammar(s),injections:s=>this._grammarRepository.injections(s)},t=[],n=this._rootScopeName,r=e.lookup(n);if(r){let s=r.injections;if(s)for(let a in s)Cn(t,a,s[a],this,r);let i=this._grammarRepository.injections(n);i&&i.forEach(a=>{let o=this.getExternalGrammar(a);if(o){let c=o.injectionSelector;c&&Cn(t,c,o,this,o)}})}return t.sort((s,i)=>s.priority-i.priority),t}getInjections(){return this._injections===null&&(this._injections=this._collectInjections()),this._injections}registerRule(e){let t=++this._lastRuleId,n=e(t);return this._ruleId2desc[t]=n,n}getRule(e){return this._ruleId2desc[e]}getExternalGrammar(e,t){if(this._includedGrammars[e])return this._includedGrammars[e];if(this._grammarRepository){let n=this._grammarRepository.lookup(e);if(n)return this._includedGrammars[e]=_n(n,t&&t.$base),this._includedGrammars[e]}}tokenizeLine(e,t,n=0){let r=this._tokenize(e,t,!1,n);return{tokens:r.lineTokens.getResult(r.ruleStack,r.lineLength),ruleStack:r.ruleStack,stoppedEarly:r.stoppedEarly}}tokenizeLine2(e,t,n=0){let r=this._tokenize(e,t,!0,n);return{tokens:r.lineTokens.getBinaryResult(r.ruleStack,r.lineLength),ruleStack:r.ruleStack,stoppedEarly:r.stoppedEarly}}_tokenize(e,t,n,r){this._rootId===-1&&(this._rootId=Ln.getCompiledRuleId(this._grammar.repository.$self,this,this._grammar.repository),this.getInjections());let s;if(!t||t===pt.NULL){s=!0;let u=this._basicScopeAttributesProvider.getDefaultAttributes(),l=this.themeProvider.getDefaults(),p=ve.set(0,u.languageId,u.tokenType,null,l.fontStyle,l.foregroundId,l.backgroundId),h=this.getRule(this._rootId).getName(null,null),d;h?d=le.createRootAndLookUpScopeName(h,p,this):d=le.createRoot("unknown",p),t=new pt(null,this._rootId,-1,-1,!1,null,d,d)}else s=!1,t.reset();e=e+`
`;let i=this.createOnigString(e),a=i.content.length,o=new mi(n,e,this._tokenTypeMatchers,this.balancedBracketSelectors),c=Tn(this,i,s,0,t,o,!0,r);return Nn(i),{lineLength:a,lineTokens:o,ruleStack:c.stack,stoppedEarly:c.stoppedEarly}}};function _n(e,t){return e=Ls(e),e.repository=e.repository||{},e.repository.$self={$vscodeTextmateLocation:e.$vscodeTextmateLocation,patterns:e.patterns,name:e.scopeName},e.repository.$base=t||e.repository.$self,e}var le=class L{constructor(t,n,r){this.parent=t,this.scopePath=n,this.tokenAttributes=r}static fromExtension(t,n){let r=t,s=t?.scopePath??null;for(let i of n)s=rt.push(s,i.scopeNames),r=new L(r,s,i.encodedTokenAttributes);return r}static createRoot(t,n){return new L(null,new rt(null,t),n)}static createRootAndLookUpScopeName(t,n,r){let s=r.getMetadataForScope(t),i=new rt(null,t),a=r.themeProvider.themeMatch(i),o=L.mergeAttributes(n,s,a);return new L(null,i,o)}get scopeName(){return this.scopePath.scopeName}toString(){return this.getScopeNames().join(" ")}equals(t){return L.equals(this,t)}static equals(t,n){do{if(t===n||!t&&!n)return!0;if(!t||!n||t.scopeName!==n.scopeName||t.tokenAttributes!==n.tokenAttributes)return!1;t=t.parent,n=n.parent}while(!0)}static mergeAttributes(t,n,r){let s=-1,i=0,a=0;return r!==null&&(s=r.fontStyle,i=r.foregroundId,a=r.backgroundId),ve.set(t,n.languageId,n.tokenType,null,s,i,a)}pushAttributed(t,n){if(t===null)return this;if(t.indexOf(" ")===-1)return L._pushAttributed(this,t,n);let r=t.split(/ /g),s=this;for(let i of r)s=L._pushAttributed(s,i,n);return s}static _pushAttributed(t,n,r){let s=r.getMetadataForScope(n),i=t.scopePath.push(n),a=r.themeProvider.themeMatch(i),o=L.mergeAttributes(t.tokenAttributes,s,a);return new L(t,i,o)}getScopeNames(){return this.scopePath.getSegments()}getExtensionIfDefined(t){let n=[],r=this;for(;r&&r!==t;)n.push({encodedTokenAttributes:r.tokenAttributes,scopeNames:r.scopePath.getExtensionIfDefined(r.parent?.scopePath??null)}),r=r.parent;return r===t?n.reverse():void 0}},pt=class X{constructor(t,n,r,s,i,a,o,c){this.parent=t,this.ruleId=n,this.beginRuleCapturedEOL=i,this.endRule=a,this.nameScopesList=o,this.contentNameScopesList=c,this.depth=this.parent?this.parent.depth+1:1,this._enterPos=r,this._anchorPos=s}_stackElementBrand=void 0;static NULL=new X(null,0,0,0,!1,null,null,null);_enterPos;_anchorPos;depth;equals(t){return t===null?!1:X._equals(this,t)}static _equals(t,n){return t===n?!0:this._structuralEquals(t,n)?le.equals(t.contentNameScopesList,n.contentNameScopesList):!1}static _structuralEquals(t,n){do{if(t===n||!t&&!n)return!0;if(!t||!n||t.depth!==n.depth||t.ruleId!==n.ruleId||t.endRule!==n.endRule)return!1;t=t.parent,n=n.parent}while(!0)}clone(){return this}static _reset(t){for(;t;)t._enterPos=-1,t._anchorPos=-1,t=t.parent}reset(){X._reset(this)}pop(){return this.parent}safePop(){return this.parent?this.parent:this}push(t,n,r,s,i,a,o){return new X(this,t,n,r,s,i,a,o)}getEnterPos(){return this._enterPos}getAnchorPos(){return this._anchorPos}getRule(t){return t.getRule(this.ruleId)}toString(){let t=[];return this._writeString(t,0),"["+t.join(",")+"]"}_writeString(t,n){return this.parent&&(n=this.parent._writeString(t,n)),t[n++]=`(${this.ruleId}, ${this.nameScopesList?.toString()}, ${this.contentNameScopesList?.toString()})`,n}withContentNameScopesList(t){return this.contentNameScopesList===t?this:this.parent.push(this.ruleId,this._enterPos,this._anchorPos,this.beginRuleCapturedEOL,this.endRule,this.nameScopesList,t)}withEndRule(t){return this.endRule===t?this:new X(this.parent,this.ruleId,this._enterPos,this._anchorPos,this.beginRuleCapturedEOL,t,this.nameScopesList,this.contentNameScopesList)}hasSameRuleAs(t){let n=this;for(;n&&n._enterPos===t._enterPos;){if(n.ruleId===t.ruleId)return!0;n=n.parent}return!1}toStateStackFrame(){return{ruleId:this.ruleId,beginRuleCapturedEOL:this.beginRuleCapturedEOL,endRule:this.endRule,nameScopesList:this.nameScopesList?.getExtensionIfDefined(this.parent?.nameScopesList??null)??[],contentNameScopesList:this.contentNameScopesList?.getExtensionIfDefined(this.nameScopesList)??[]}}static pushFrame(t,n){let r=le.fromExtension(t?.nameScopesList??null,n.nameScopesList);return new X(t,n.ruleId,n.enterPos??-1,n.anchorPos??-1,n.beginRuleCapturedEOL,n.endRule,r,le.fromExtension(r,n.contentNameScopesList))}},gi=class{balancedBracketScopes;unbalancedBracketScopes;allowAny=!1;constructor(e,t){this.balancedBracketScopes=e.flatMap(n=>n==="*"?(this.allowAny=!0,[]):Ne(n,Le).map(r=>r.matcher)),this.unbalancedBracketScopes=t.flatMap(n=>Ne(n,Le).map(r=>r.matcher))}get matchesAlways(){return this.allowAny&&this.unbalancedBracketScopes.length===0}get matchesNever(){return this.balancedBracketScopes.length===0&&!this.allowAny}match(e){for(let t of this.unbalancedBracketScopes)if(t(e))return!1;for(let t of this.balancedBracketScopes)if(t(e))return!0;return this.allowAny}},mi=class{constructor(e,t,n,r){this.balancedBracketSelectors=r,this._emitBinaryTokens=e,this._tokenTypeOverrides=n,this._lineText=null,this._tokens=[],this._binaryTokens=[],this._lastTokenEndIndex=0}_emitBinaryTokens;_lineText;_tokens;_binaryTokens;_lastTokenEndIndex;_tokenTypeOverrides;produce(e,t){this.produceFromScopes(e.contentNameScopesList,t)}produceFromScopes(e,t){if(this._lastTokenEndIndex>=t)return;if(this._emitBinaryTokens){let r=e?.tokenAttributes??0,s=!1;if(this.balancedBracketSelectors?.matchesAlways&&(s=!0),this._tokenTypeOverrides.length>0||this.balancedBracketSelectors&&!this.balancedBracketSelectors.matchesAlways&&!this.balancedBracketSelectors.matchesNever){let i=e?.getScopeNames()??[];for(let a of this._tokenTypeOverrides)a.matcher(i)&&(r=ve.set(r,0,a.type,null,-1,0,0));this.balancedBracketSelectors&&(s=this.balancedBracketSelectors.match(i))}if(s&&(r=ve.set(r,0,8,s,-1,0,0)),this._binaryTokens.length>0&&this._binaryTokens[this._binaryTokens.length-1]===r){this._lastTokenEndIndex=t;return}this._binaryTokens.push(this._lastTokenEndIndex),this._binaryTokens.push(r),this._lastTokenEndIndex=t;return}let n=e?.getScopeNames()??[];this._tokens.push({startIndex:this._lastTokenEndIndex,endIndex:t,scopes:n}),this._lastTokenEndIndex=t}getResult(e,t){return this._tokens.length>0&&this._tokens[this._tokens.length-1].startIndex===t-1&&this._tokens.pop(),this._tokens.length===0&&(this._lastTokenEndIndex=-1,this.produce(e,t),this._tokens[this._tokens.length-1].startIndex=0),this._tokens}getBinaryResult(e,t){this._binaryTokens.length>0&&this._binaryTokens[this._binaryTokens.length-2]===t-1&&(this._binaryTokens.pop(),this._binaryTokens.pop()),this._binaryTokens.length===0&&(this._lastTokenEndIndex=-1,this.produce(e,t),this._binaryTokens[this._binaryTokens.length-2]=0);let n=new Uint32Array(this._binaryTokens.length);for(let r=0,s=this._binaryTokens.length;r<s;r++)n[r]=this._binaryTokens[r];return n}},bi=class{constructor(e,t){this._onigLib=t,this._theme=e}_grammars=new Map;_rawGrammars=new Map;_injectionGrammars=new Map;_theme;dispose(){for(let e of this._grammars.values())e.dispose()}setTheme(e){this._theme=e}getColorMap(){return this._theme.getColorMap()}addGrammar(e,t){this._rawGrammars.set(e.scopeName,e),t&&this._injectionGrammars.set(e.scopeName,t)}lookup(e){return this._rawGrammars.get(e)}injections(e){return this._injectionGrammars.get(e)}getDefaults(){return this._theme.getDefaults()}themeMatch(e){return this._theme.match(e)}grammarForScopeName(e,t,n,r,s){if(!this._grammars.has(e)){let i=this._rawGrammars.get(e);if(!i)return null;this._grammars.set(e,hi(e,i,t,n,r,s,this,this._onigLib))}return this._grammars.get(e)}},Fn=class{_options;_syncRegistry;_ensureGrammarCache;constructor(e){this._options=e,this._syncRegistry=new bi(it.createFromRawTheme(e.theme,e.colorMap),e.onigLib),this._ensureGrammarCache=new Map}dispose(){this._syncRegistry.dispose()}setTheme(e,t){this._syncRegistry.setTheme(it.createFromRawTheme(e,t))}getColorMap(){return this._syncRegistry.getColorMap()}loadGrammarWithEmbeddedLanguages(e,t,n){return this.loadGrammarWithConfiguration(e,t,{embeddedLanguages:n})}loadGrammarWithConfiguration(e,t,n){return this._loadGrammar(e,t,n.embeddedLanguages,n.tokenTypes,new gi(n.balancedBracketSelectors||[],n.unbalancedBracketSelectors||[]))}loadGrammar(e){return this._loadGrammar(e,0,null,null,null)}_loadGrammar(e,t,n,r,s){let i=new Vs(this._syncRegistry,e);for(;i.Q.length>0;)i.Q.map(a=>this._loadSingleGrammar(a.scopeName)),i.processQueue();return this._grammarForScopeName(e,t,n,r,s)}_loadSingleGrammar(e){this._ensureGrammarCache.has(e)||(this._doLoadSingleGrammar(e),this._ensureGrammarCache.set(e,!0))}_doLoadSingleGrammar(e){let t=this._options.loadGrammar(e);if(t){let n=typeof this._options.getInjections=="function"?this._options.getInjections(e):void 0;this._syncRegistry.addGrammar(t,n)}}addGrammar(e,t=[],n=0,r=null){return this._syncRegistry.addGrammar(e,t),this._grammarForScopeName(e.scopeName,n,r)}_grammarForScopeName(e,t=0,n=null,r=null,s=null){return this._syncRegistry.grammarForScopeName(e,t,n,r,s)}},Ge=pt.NULL;function wi(){let e=tt({target:"ES2024"});return{createOnigScanner(t){return e.createScanner(t)},createOnigString(t){return e.createString(t)}}}function Ci(e){for(let t=e.length-1;t>=0;t-=1){let n=e[t];if(n.startsWith("invalid.")||n.startsWith("invalid"))return"typerb-token-invalid";if(n.startsWith("comment."))return"typerb-token-comment";if(n.startsWith("constant.character.escape"))return"typerb-token-escape";if(n.startsWith("constant."))return"typerb-token-constant";if(n.startsWith("keyword."))return"typerb-token-keyword";if(n.startsWith("storage."))return"typerb-token-storage";if(n.startsWith("entity.name.function")||n.startsWith("support.function"))return"typerb-token-function";if(n.startsWith("entity.name.type")||n.startsWith("support.type"))return"typerb-token-type";if(n.startsWith("variable."))return"typerb-token-variable";if(n.startsWith("string."))return"typerb-token-string"}return null}function Dn(e,t){let n=[];for(let r of t){let s=e.slice(r.startIndex,r.endIndex);if(s.length===0)continue;let i=Ci(r.scopes),a=n.at(-1);a?.className===i?a.text+=s:n.push({text:s,className:i})}return n}async function Un({grammarJson:e}){let t=JSON.parse(e),r=await new Fn({onigLib:wi(),loadGrammar(s){return s!=="source.trb"?null:t}}).loadGrammar("source.trb");if(!r)throw new Error("TypeRB TextMate grammar could not be loaded");return{initialState(){return Ge},tokenizeLine(s,i=Ge){let a=r.tokenizeLine(s,i);return{segments:Dn(s,a.tokens),ruleStack:a.ruleStack}},tokenizeSource(s){let i=Ge;return s.split(/\r\n|\n|\r/).map(a=>{let o=r.tokenizeLine(a,i);return i=o.ruleStack,Dn(a,o.tokens)})}}}async function _i({addStyles:e}={}){e?.();let t=await Un({grammarJson:gt}),n=new Map,r,s=!1,i=!1;async function a(){if(s){i=!0;return}s=!0;try{await yt({document,location,highlighter:t,cache:n})}catch(c){console.warn("[TypeRB GitHub] Highlighting failed",c)}finally{s=!1,i&&(i=!1,o())}}function o(){clearTimeout(r),r=setTimeout(a,80)}new MutationObserver(o).observe(document.documentElement,{childList:!0,subtree:!0}),document.addEventListener("turbo:load",o),document.addEventListener("pjax:end",o),window.addEventListener("popstate",o),window.addEventListener("urlchange",o),await a()}function Wn(e){_i(e).catch(t=>{console.warn("[TypeRB GitHub] Initialization failed",t)})}var dt=`
.typerb-token-comment { color: var(--color-prettylights-syntax-comment); }
.typerb-token-constant { color: var(--color-prettylights-syntax-constant); }
.typerb-token-escape { color: var(--color-prettylights-syntax-string-regexp); }
.typerb-token-function { color: var(--color-prettylights-syntax-entity); }
.typerb-token-invalid {
  color: var(--color-prettylights-syntax-brackethighlighter-unmatched);
  text-decoration: underline wavy;
}
.typerb-token-keyword { color: var(--color-prettylights-syntax-keyword); }
.typerb-token-storage { color: var(--color-prettylights-syntax-storage-modifier-import); }
.typerb-token-string { color: var(--color-prettylights-syntax-string); }
.typerb-token-type { color: var(--color-prettylights-syntax-entity); }
.typerb-token-variable { color: var(--color-prettylights-syntax-variable); }
`;function Si(){if(typeof GM_addStyle=="function"){GM_addStyle(dt);return}let e=document.createElement("style");e.textContent=dt,document.head.append(e)}Wn({addStyles:Si});})();
