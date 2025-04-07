# Cursor Rules 整理工具

这个项目用于整理和收集来自不同项目的Cursor规则文件，目前起步阶段，后续将扩展到更多编程语言。

## 项目简介

Cursor是一款AI辅助编程工具，它使用规则文件来指导其行为。本项目的目标是收集、整理和标准化这些规则文件，使开发者能够更有效地利用Cursor进行开发工作。

## 当前支持的编程语言

- **Python**
- **Vue**

## 功能特点

- 收集不同项目中的Cursor规则文件
- 对规则文件进行分类和整理
- 提供标准化的规则模板
- 便于在不同项目间共享和复用规则

## 使用方法

1. 克隆本仓库
2. 浏览相应语言和框架的规则
3. 将适用的规则应用到您的项目中
4. 贡献您自己的规则回馈社区

## 项目结构

```
cursor-rules/
├── python/              # Python项目的Cursor规则
│   ├── django/          # Django框架相关规则
│   ├── flask/           # Flask框架相关规则
│   └── fastapi/         # FastAPI框架相关规则
├── javascript/          # JavaScript项目的规则
│   ├── react/           # React框架相关规则
│   └── node/            # Node.js相关规则
├── java/                # Java项目的规则
│   └── spring/          # Spring框架相关规则
├── go/                  # Go项目的规则
├── rust/                # Rust项目的规则
├── vue/                 # Vue框架相关规则
└── templates/           # 规则模板
```

## 贡献指南

欢迎提交Pull Request来分享您的Cursor规则。请确保您的规则文件遵循项目的命名约定和结构。规则可以使用Markdown(.mdc)或JSON格式。

## 许可证

MIT