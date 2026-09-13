# React + TypeScript + Vite

## 在 Windows 浏览器访问

先确认 Windows 电脑和树莓派在同一个局域网，并将树莓派后端地址配置为实际地址。在 PowerShell 中运行：

```powershell
cd frontend
$env:VITE_PI_URL = "http://192.168.2.91:8081"
npm run dev -- --host 0.0.0.0
```

终端会显示开发机的局域网 IP，例如 `192.168.2.20`。然后在 Windows 浏览器打开：

```text
https://192.168.2.20:5173
```

首次打开时浏览器会提示开发证书不受信任，选择“高级”后继续访问即可。如果树莓派地址变化，只需修改 `VITE_PI_URL`。也可以复制 `.env.example` 为 `.env.local` 后填写地址。

This template provides a minimal setup to get React working in Vite with HMR and some Oxlint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Oxc](https://oxc.rs)
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/)

## React Compiler

The React Compiler is not enabled on this template because of its impact on dev & build performances. To add it, see [this documentation](https://react.dev/learn/react-compiler/installation).

## Expanding the Oxlint configuration

If you are developing a production application, we recommend enabling type-aware lint rules by installing `oxlint-tsgolint` and editing `.oxlintrc.json`:

```json
{
  "$schema": "./node_modules/oxlint/configuration_schema.json",
  "plugins": ["react", "typescript", "oxc"],
  "options": {
    "typeAware": true
  },
  "rules": {
    "react/rules-of-hooks": "error",
    "react/only-export-components": ["warn", { "allowConstantExport": true }]
  }
}
```

See the [Oxlint rules documentation](https://oxc.rs/docs/guide/usage/linter/rules) for the full list of rules and categories.
