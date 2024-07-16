import { IContextualMenuStyles } from "@fluentui/react"
import { S } from "../core"
import { border, cssVar } from "../theme"

// https://github.com/bevacqua/fuzzysearch/blob/master/index.js
export function fuzzysearch(haystack: S, needle: S) {
  haystack = haystack.toLowerCase()
  needle = needle.toLowerCase()
  return haystack.includes(needle);
}

// https://github.com/h2oai/wave/issues/1395.
export const fixMenuOverflowStyles: Partial<IContextualMenuStyles> = {
  list: {
    border: border(1, cssVar('$neutralQuaternaryAlt')),
    '.ms-ContextualMenu-link': { lineHeight: 'unset' },
    '.ms-ContextualMenu-submenuIcon': { lineHeight: 'unset', display: 'flex', alignItems: 'center' },
  }
}