# M0 命名核查报告（llmberth · 2026-09-08）

## 结论

**定名 `llmberth`**。go module path：`github.com/THY17308111153/llmberth`（用户当前账号，后续可迁组织）。CLI 二进制/命令名：`llmberth`。

## 核查矩阵

| 候选 | GitHub 精确同名 | 同赛道风险 | 域名粗查（.com/.dev/.io） | 全网产品级 | 结论 |
|------|----------------|-----------|--------------------------|-----------|------|
| **llmberth** | 零 | 零 | 全部可得（DNS 无解析） | 无同名 LLM 项目 | ✅ **选用** |
| llmwharf | 零 | 零 | 全部可得 | 无同名 | 备选（wharf=码头，同隐喻） |
| llmyard | 零 | 零 | 仅 .com 被占（有解析） | 无同名；发音与 "lanyard" 混淆 | 弃（口碑传播受损） |
| llmkiln | 零（字符串） | **高**：Kiln-AI/Kiln 5047★（LLM 工作台，同赛道核心词） | 全部可得 | 无同名 | 弃 |
| drydock | 多个同名（venmo/DryDock-iOS 431★、CodesWhat/drydock 252★ 容器运维同赛道） | 高 | — | — | 弃 |
| lathe | devenjarvis/lathe 1662★（LLM 内容生成） | 高 | — | — | 弃 |
| winch | 无精确同名，但通用词、品牌弱 | 中 | — | — | 弃 |
| llmdock | 零 | 低 | .com 被占 | — | 弃 |

## 验收（对照 PLAN.md M0）

- [x] 同名同赛道冲突为零（GitHub 精确匹配 + 全网搜索）
- [x] 域名可得（llmberth.com / llmberth.dev / llmberth.io 三者 DNS 均未解析，粗查可注册；正式注册在发布前完成）
- [x] go module path 确定

## 后续动作

- 社媒句柄（GitHub org / X）预计可得，发布前注册；本报告为 DNS 粗查结论，注册动作不属于 M0 范围。
- 密钥前缀采用 `lbt_live_`（原 `lsk_live_` 为旧代号缩写，随定名一并替换）。
- 项目清单文件采用 `.llmberth.yaml`（同上）。
