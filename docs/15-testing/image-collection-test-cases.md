# Image Collection Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Image Collections |
|-----|------------|--------------------|
| Lato Enquiry Management | 11.4.0 | <N> |
| Evora - Factory Management | 10.24.15 | <N> |
| Lato Product Inventory | 11.2.0 | <N> |

---

## Setup

### 1. Build mxcli

```bash
make build && make test && make lint-go
```

### 2. Prepare test images

```bash
mkdir -p /tmp/mxcli-test-images
convert -size 32x32 xc:red /tmp/mxcli-test-images/logo.png
convert -size 32x32 xc:blue /tmp/mxcli-test-images/icon.png
echo '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"/>' > /tmp/mxcli-test-images/vector.svg
```

### 3. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder.
> Dropped image collections cannot be recovered.
>
> ```bash
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW IMAGE COLLECTIONS

### 1.1 List all image collections

```
show image collections;
```

**Expected:** Table with columns `Image Collection | Export Level | Images`. Summary `(N image collections)`.

### 1.2 Filter by module

```
show image collections in Administration;
```

**Expected:** Only image collections from `Administration` module.

### 1.3 Empty module

```
show image collections in NonExistentModule;
```

**Expected:** Empty result or `(0 image collections)`.

---

## 2. DESCRIBE IMAGE COLLECTION

### 2.1 Basic describe

```
describe image collection MyModule.Icons;
```

**Expected:**
```
create or replace image collection MyModule.Icons export level 'Public' (
  image logo from file 'logo.png',
  image icon from file 'icon.svg'
);
/
```

### 2.2 Image extraction to disk

```
describe image collection MyModule.Icons;
```

**Expected:** Images extracted to `/tmp/mxcli-preview/MyModule.Icons/`. Each image file present with original format.

### 2.3 Supported formats

Verify `describe` correctly identifies format for each type:

| Extension | Format |
|-----------|--------|
| `.png` | Png (default) |
| `.svg` | Svg |
| `.gif` | Gif |
| `.jpg` | Jpg |
| `.bmp` | Bmp |
| `.webp` | Webp |

### 2.4 Non-existent image collection

```
describe image collection Fake.Missing;
```

**Expected:** Error — image collection not found.

---

## 3. CREATE IMAGE COLLECTION

### 3.1 Empty collection

```
create image collection MyModule.EmptyIcons;
```

**Expected:** `Created image collection: MyModule.EmptyIcons`. `show image collections in MyModule` lists it with `Images: 0`.

### 3.2 Collection with images

```
create image collection MyModule.AppIcons export level 'Public' comment 'Application icons' (
  image logo from file '/tmp/mxcli-test-images/logo.png',
  image icon from file '/tmp/mxcli-test-images/vector.svg'
);
```

**Expected:** Created with 2 images. `describe` shows both images with correct formats.

### 3.3 Hidden export level (default)

```
create image collection MyModule.HiddenIcons (
  image logo from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** Created. `show image collections` shows `Export Level: Hidden`.

### 3.4 Public export level

```
create image collection MyModule.PublicIcons export level 'Public' (
  image logo from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** Created. `show image collections` shows `Export Level: Public`.

### 3.5 Format auto-detection from extension

Create collections with each supported format. Verify `describe` output shows correct format.

| File | Detected Format |
|------|-----------------|
| `image.png` | Png |
| `image.svg` | Svg |
| `image.gif` | Gif |
| `image.jpg` | Jpg |
| `image.bmp` | Bmp |
| `image.webp` | Webp |

### 3.6 `create or replace` — new collection

```
create or replace image collection MyModule.Fresh (
  image a from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** Creates collection (same as without `or replace`).

### 3.7 `create or replace` — existing collection

```
create or replace image collection MyModule.Fresh (
  image a from file '/tmp/mxcli-test-images/logo.png',
  image b from file '/tmp/mxcli-test-images/icon.png'
);
```

**Expected:** Replaces existing collection. `describe` shows 2 images.

### 3.8 Duplicate (without `or replace`)

```
create image collection MyModule.Fresh (
  image x from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** Error — image collection already exists.

### 3.9 Module auto-creation

```
create image collection NewModule.Icons (
  image logo from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** Both module and image collection created.

---

## 4. DROP IMAGE COLLECTION

### 4.1 Drop existing collection

```
drop image collection MyModule.AppIcons;
```

**Expected:** `Dropped image collection: MyModule.AppIcons`.

### 4.2 Drop non-existent collection

```
drop image collection MyModule.Fake;
```

**Expected:** Error — not found.

---

## 5. ROUNDTRIP

### 5.1 Create → Describe → Recreate

1. Create collection with images (3.2)
2. `describe image collection MyModule.AppIcons` — save output
3. `drop image collection MyModule.AppIcons`
4. Execute saved describe output (change `create or replace` to `create`)
5. `describe` again

**Expected:** Output identical between step 2 and step 5. Extracted image files match.

---

## 6. FAILURE MODES

### 6.1 Not connected

Run any command without `-p` or `connect`.

**Expected:** Error — not connected to a project.

### 6.2 File not found

```
create image collection MyModule.Bad (
  image missing from file '/tmp/does-not-exist.png'
);
```

**Expected:** Error — file not found.

### 6.3 Invalid image format

```
create image collection MyModule.Bad (
  image broken from file '/tmp/mxcli-test-images/notanimage.txt'
);
```

**Expected:** Error — unsupported image format.

### 6.4 Backend create failure

Simulate backend failure (e.g., corrupt MPR, disk full).

**Expected:** Error from backend layer with descriptive message.

### 6.5 Preview directory creation failure

If `/tmp/mxcli-preview/` is not writable:

```
describe image collection MyModule.Icons;
```

**Expected:** Error creating preview directory.

---

## 7. BOUNDARY & STRESS

### 7.1 Large image file

Create a collection with a large image (10 MB+ PNG).

```
create image collection MyModule.LargeIcons (
  image big from file '/tmp/mxcli-test-images/large-10mb.png'
);
```

**Expected:** Collection created. `describe` extracts image correctly. Verify file size preserved.

### 7.2 Many images in one collection

Create a collection with 50+ images.

```
create image collection MyModule.ManyIcons (
  image img001 from file '/tmp/mxcli-test-images/logo.png',
  image img002 from file '/tmp/mxcli-test-images/icon.png',
  ...
  image img050 from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** All 50 images stored. `show image collections` reports correct count. `describe` extracts all files.

### 7.3 Image with special characters in name

```
create image collection MyModule.SpecialIcons (
  image my_icon_v2 from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** Image name with underscores accepted. Verify no name sanitization issues.

### 7.4 Empty image file

Create a 0-byte file and attempt to import:

```bash
touch /tmp/mxcli-test-images/empty.png
```

```
create image collection MyModule.EmptyFile (
  image empty from file '/tmp/mxcli-test-images/empty.png'
);
```

**Expected:** Error or warning about empty/invalid image data.

### 7.5 Duplicate image names in one collection

```
create image collection MyModule.DupeIcons (
  image logo from file '/tmp/mxcli-test-images/logo.png',
  image logo from file '/tmp/mxcli-test-images/icon.png'
);
```

**Expected:** Error — duplicate image name within collection.

### 7.6 All supported formats in one collection

```
create image collection MyModule.AllFormats export level 'Public' (
  image png_img from file '/tmp/mxcli-test-images/test.png',
  image svg_img from file '/tmp/mxcli-test-images/test.svg',
  image gif_img from file '/tmp/mxcli-test-images/test.gif',
  image jpg_img from file '/tmp/mxcli-test-images/test.jpg',
  image bmp_img from file '/tmp/mxcli-test-images/test.bmp',
  image webp_img from file '/tmp/mxcli-test-images/test.webp'
);
```

**Expected:** All 6 formats accepted. `describe` shows correct format for each.

### 7.7 Replace collection with different image set

```
create or replace image collection MyModule.Evolving (
  image v1 from file '/tmp/mxcli-test-images/logo.png'
);
create or replace image collection MyModule.Evolving (
  image v2a from file '/tmp/mxcli-test-images/icon.png',
  image v2b from file '/tmp/mxcli-test-images/logo.png'
);
```

**Expected:** Second `create or replace` fully replaces. `describe` shows only `v2a` and `v2b`.

---

## Test Project Coverage Matrix

| Operation | Lato Enquiry | Evora Factory | Lato Inventory |
|-----------|:---:|:---:|:---:|
| SHOW IMAGE COLLECTIONS | x | x | x |
| DESCRIBE IMAGE COLLECTION | x | x | x |
| CREATE IMAGE COLLECTION | x | | |
| DROP IMAGE COLLECTION | x | | |
| ROUNDTRIP | x | | |
| BOUNDARY & STRESS | x | | |

---

## Automated Test Coverage

| Section | Automated | Manual-only |
|---------|:---------:|:-----------:|
| 1. SHOW | Mock tests | |
| 2. DESCRIBE | Mock tests | Image extraction to disk |
| 3. CREATE | Mock tests | Format auto-detection |
| 4. DROP | Mock tests | |
| 5. Roundtrip | | All manual |
| 6. Failure modes | Partial | Backend, preview dir |
| 7. Boundary & stress | | All manual |

---

## Manual Test Report Template

**Tester:** _______________
**Date:** _______________
**Project:** _______________

| # | Section | Test | Pass | Fail | Skip | Notes |
|---|---------|------|:----:|:----:|:----:|-------|
| 1.1 | SHOW | List all | | | | |
| 1.2 | SHOW | Filter module | | | | |
| 1.3 | SHOW | Empty module | | | | |
| 2.1 | DESCRIBE | Basic | | | | |
| 2.2 | DESCRIBE | Image extraction | | | | |
| 2.3 | DESCRIBE | Supported formats | | | | |
| 2.4 | DESCRIBE | Not found | | | | |
| 3.1 | CREATE | Empty collection | | | | |
| 3.2 | CREATE | With images | | | | |
| 3.3 | CREATE | Hidden (default) | | | | |
| 3.4 | CREATE | Public export level | | | | |
| 3.5 | CREATE | Format auto-detection | | | | |
| 3.6 | CREATE | create or replace (new) | | | | |
| 3.7 | CREATE | create or replace (update) | | | | |
| 3.8 | CREATE | Duplicate error | | | | |
| 3.9 | CREATE | Module auto-creation | | | | |
| 4.1 | DROP | Existing | | | | |
| 4.2 | DROP | Non-existent | | | | |
| 5.1 | ROUNDTRIP | Create → Describe → Recreate | | | | |
| 6.1 | FAILURE | Not connected | | | | |
| 6.2 | FAILURE | File not found | | | | |
| 6.3 | FAILURE | Invalid format | | | | |
| 6.4 | FAILURE | Backend create failure | | | | |
| 6.5 | FAILURE | Preview dir failure | | | | |
| 7.1 | BOUNDARY | Large image (10 MB+) | | | | |
| 7.2 | BOUNDARY | Many images (50+) | | | | |
| 7.3 | BOUNDARY | Special chars in name | | | | |
| 7.4 | BOUNDARY | Empty image file | | | | |
| 7.5 | BOUNDARY | Duplicate image names | | | | |
| 7.6 | BOUNDARY | All formats in one | | | | |
| 7.7 | BOUNDARY | Replace with different set | | | | |

**Summary:** ___ / ___ passed | ___ failed | ___ skipped
