# Sub2API-Jack 共用開發與發佈流程

本文件適用於人類開發者及任何 coding agent／harness。流程以本 repo 和 GitHub
設定為準，不依賴特定工具、之前的對話或私人記憶。功能與部署的技術細節見
[JACK-DEVELOPMENT.md](docs/JACK-DEVELOPMENT.md)。

## 權限與維護者

- 這是 `jacklee-code` 個人維護的公開二次開發版，**不接受外部 PR**。
  GitHub 的 `pull_request_creation_policy` 設為 `collaborators_only`，目前唯一
  有寫入／管理權限的人是 `jacklee-code`。不要在一般開發工作中新增協作者或擴大權限。
- 只有 `jacklee-code`，以及在其授權會話中使用該身份的 agent，可以推送、決定合併
  及觸發發佈。repo 自己的 GitHub Actions 使用受限 `GITHUB_TOKEN`，代為執行
  已設定的上游同步、合併及發佈；這不代表第三方獲得發佈權。
- 公開 clone／fork 只提供原始碼，不授予本 repo、GHCR 或 VPS 的寫入權限。
  發佈與同步 workflow 限定原 repo、`main` 及 owner／內建 Actions bot 身份。
- `main` 要求 `Jack verified` 並保持與 base 最新狀態，禁止 force push／刪除。
  不要為通過流程而關閉檢查或改寫已發布 tag。
- 文件是操作指引，實際限制由 GitHub 權限、PR creation policy、branch protection
  和 workflow 條件執行；文件本身不授權任何操作。

## 新環境開始工作

```sh
git clone https://github.com/jacklee-code/Sub2API-Jack.git
cd Sub2API-Jack
git remote add upstream https://github.com/Wei-Shaw/sub2api.git
gh repo set-default jacklee-code/Sub2API-Jack
git switch -c feat/your-feature
```

已存在的 checkout 先查看 `git status`、保留未提交工作，再更新自己的 `main` 並開
功能分支。`origin` 是 Jack repo；`upstream` 是官方原始碼。GitHub CLI 操作若有歧義，
明確指定 `--repo jacklee-code/Sub2API-Jack`。推送前確認登入身份為 `jacklee-code`。

使用 `backend/go.mod` 指定的 Go 版本；CI 目前使用 Node 20、pnpm 9。
整合測試需要 Docker。使用隔離 PostgreSQL／Redis 與合成使用者資料，普通功能開發
不需要正式站憑證或 SSH 存取，也不得把測試指向正式資料庫。

## 功能開發至上線

| 階段 | 人／agent 要做的事 | GitHub 自動執行的事 |
| --- | --- | --- |
| 開發 | 從最新 `main` 開分支、實作、執行適當驗證、更新受影響文件 | 無 |
| PR | 推送分支，在本 repo 開 PR，說明改動、驗證與部署影響 | 執行 `Jack checks`，回報 `Jack verified` |
| 合併 | `jacklee-code` 或其已授權 agent 檢查結果並合併；一般功能 PR 不會自行合併 | 若 owner 對該 PR 啟用 auto-merge，等待必要檢查成功後合併 |
| 發佈 | 查看 `Publish Jack` 是否成功，失敗時修復 | `main` 更新後再次驗證，建立版本、Release、校驗檔與 AMD64／ARM64 GHCR 映像 |
| 正式部署 | 操作者在後台觸發更新；執行環境改變時使用映像切換腳本，驗證健康及資料 | 發佈本身不會更新 VPS；只有操作者觸發後才執行更新腳本 |

採用 **merge** 維護上游與 Jack 的歷史；不要 rebase／force-push 已公開或共用的分支。
使用 PR 完成正常開發，不要直接把官方版本覆蓋到自己的 `main`。

僅 push 功能分支或開 PR 不會發佈。**任何合併到 `main` 的改動，包括純文件改動，
都會觸發發佈。** 版本 `v<官方版本>-jack.N`、tag、checksums、`jack-release.json`
及映像均由 workflow 產生，不需 agent 手工編號或重複打包。

## 驗證

依改動範圍執行有意義的驗證；必要的完整 CI 以
[jack-checks.yml](.github/workflows/jack-checks.yml) 為準：

```sh
(cd backend && go test -tags=unit ./...)
(cd backend && go test -tags=integration ./...)
(cd frontend && pnpm install --frozen-lockfile)
(cd frontend && pnpm run lint:check && pnpm run test:run && pnpm run build)
python3 -m unittest discover -s scripts/jack -p 'test_*.py'
bash -n deploy/jack/switch-image.sh
sh -n deploy/jack/entrypoint.sh
git diff --check
```

文件改動在本機檢查連結、路徑及與實作的一致性即可；GitHub 仍執行必要檢查。
UI 改動驗證實際畫面與關鍵操作；訂閱／車隊／更新改動驗證資料保留及失敗路徑。
交付時說明哪些檢查已通過、未執行或受環境限制，不能把「PR 已合併」當成
「Release 已成功」或「正式站已更新」。

## 官方上游更新

[jack-sync.yml](.github/workflows/jack-sync.yml) 每小時第 17 分鐘檢查官方 stable
Release，透過 [sync-upstream.py](scripts/jack/sync-upstream.py) 建立 merge PR，
驗證該次 commit，成功後自動合併並觸發發佈。這是**上游同步 PR 的特別流程**，
不會替所有功能 PR 啟用自動合併。

唯讀檢查可使用 `python3 scripts/jack/sync-upstream.py --dry-run`。
`.jack/upstream.json` 必須記錄實際已合併的官方 tag／commit，不能只改版本號。

若唯一衝突是 `frontend/pnpm-lock.yaml`，排程保留 Jack 版本並以
`pnpm install --lockfile-only` 依合併後的 `package.json` 重建，再照常驗證；
過期的 lockfile 會在 `--frozen-lockfile` 安裝時失敗，不會被合併。

衝突或測試失敗時，現有 PR 會保留等待修復；排程不會自動解決語意衝突。
新的 sync PR 會指派給 `jacklee-code`；衝突時該次 workflow 會失敗以觸發 GitHub
通知，之後每次排程在草稿仍未修復時留下 warning。沒有 warning 的綠色同步執行代表沒有待處理衝突。
在原 sync 分支讀取 `.jack/upstream-conflict.md`（如有），merge 記錄的上游 commit，
保留 Jack 功能，更新 `.jack/upstream.json`，移除已解決的衝突標記，推送並重新驗證。
因排程會保留既有 PR，修復後由 owner／已授權 agent 完成合併。

若透過 workflow 的 `GITHUB_TOKEN` 合併，須像 `jack-sync.yml` 一樣明確 dispatch
[jack-main.yml](.github/workflows/jack-main.yml)，不能依賴該 token 的 push 再觸發
另一個 workflow。發佈失敗應先診斷、修復並重跑；不可覆寫已發布的版本。

## 部署、回退與維護

功能開發、發佈及正式部署是不同工作，按使用者在本次任務中給予的範圍執行。
已有明確授權就繼續，不需每一步重複確認；clone repo 本身不包含部署授權。

- 後台左上角更新來源為 **Sub2API-Jack 自己的 Release**。同 runtime 可更新持久化
  執行檔 `/app/data/jack-runtime/sub2api`；一般容器重建會保留該版本。
- runtime 指紋改變時，在真正的 Compose 部署目錄使用
  [switch-image.sh](deploy/jack/switch-image.sh) 與選定 Jack 版本。
- 更新前確認備份、實際版本與部署位置；更新後檢查健康、成員、期限、額度及 API keys。
  保留校驗、manifest、runtime 相容性與備份檢查，不改用會移除 Jack 功能的官方映像。
- 回退受版本相容性限制；資料庫還原會回溯寫入，必須依
  [部署與回復指南](docs/JACK-DEVELOPMENT.md#deployment-and-recovery) 另行維護。
- 正式主機 runbook、SSH 憑證、私有 `.env`、每日備份 wrapper 與異機備份金鑰不在公開
  repo。需要維護正式站時取得當前 host runbook；不可根據聊天歷史猜測即時部署狀態。

## 讓不同工具讀取同一流程

人類開發者由 README 進入本文件。支援 `AGENTS.md` 的 agent 由
[AGENTS.md](AGENTS.md) 進入；[CLAUDE.md](CLAUDE.md) 只是相容入口，以
`@AGENTS.md` 匯入相同規則，沒有另一套 Claude 專用發佈流程。
其匯入行為見 [Claude Code 文件](https://code.claude.com/docs/en/memory#import-additional-files)。

工具是否自動讀取入口取決於版本及設定，不能保證所有 harness 都會自動載入。
通用的開場指示是：「先閱讀 `CONTRIBUTING.md`、`AGENTS.md` 及
`docs/JACK-DEVELOPMENT.md`，按 repo 的流程完成這次工作。」
新增其他工具入口時引用上述文件，避免複製規則造成分歧。
