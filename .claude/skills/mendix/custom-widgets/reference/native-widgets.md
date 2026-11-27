# Native mobile widgets

Supporting reference for [custom-widgets](../SKILL.md).

The 39 widgets of Native Mobile Resources (`nativegallery`, `nativeslider`, `nativebottomsheet`, `nativebarchart`, …) need no template in mxcli. After `widget init`, each one is built from the project's own `.mpk` file.

- **Set what the widget requires.** `check` warns (MDL-WIDGET56) about each required setting left out: a gallery's data source, a slider's attribute, a repeater's `template { … }` content, a chart's series. mxbuild fails the build on each with CE0642.
- **Use them on native pages only.** `check` warns (MDL-WIDGET55) about a native widget on a web page, and refuses a web widget (`combobox`, `datagrid`, `image`, …) on a native page: mxbuild fails it with CE6089. For an image there, write `staticimage` or `dynamicimage`.
- **Slot names come from the definition.** A native gallery's item slot is `template { … }`, not `content`; `check` lists a widget's slots when one is wrong (MDL-WIDGET26).
- **Slider, Range slider, Rating, Toggle buttons:** `editable:` is the widget's own setting, `default` or `never`. It is not the core `Editable`.

```sql
nativegallery g1 (datasource: database from Module.Item, phoneColumns: 2) {
  template { dynamictext t1 (Content: '{1}', ContentParams: ({1} = Title)) }
}
nativeslider s1 (valueAttribute: Score, editable: never)
```
