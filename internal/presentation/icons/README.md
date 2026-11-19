# 界面图标

`icons.go` 是**生成文件**，由 [Lucide](https://lucide.dev) 的 SVG 编译成
`gfx.Icon`（纯几何，运行时不解析 SVG、不带字体）。

- `svg/` 是图标源文件（Lucide，ISC 许可，见下）
- 重新生成（新增图标时把对应 SVG 放进 `svg/` 再执行）：

```bash
bash scripts/icons.sh
```

脚本在临时模块里调用 Viva 的图标编译器，避免把编译器自身的依赖写进本仓库的
`go.sum`。

## 许可

图标来自 [lucide-icons/lucide](https://github.com/lucide-icons/lucide)，
ISC License，Copyright (c) for portions of Lucide are held by Cole Bemis 2013-2022
as part of Feather (MIT). All other copyright (c) for Lucide are held by Lucide
Contributors 2022.
