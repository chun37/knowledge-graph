# kg — 小さなナレッジグラフ CLI

Go の標準ライブラリだけで書かれた、シングルバイナリのナレッジグラフ管理ツール。

## ナレッジグラフとは

**ナレッジグラフ (Knowledge Graph)** とは、現実世界の事物や概念を **ノード（エンティティ）**、それらの関係を **エッジ（リレーション）** として表現する **有向ラベル付きグラフ** によって知識を構造化したもの。

- 基本単位は **トリプル**: `(主語, 述語, 目的語)` = `(Subject, Predicate, Object)`
  - 例: `(夏目漱石, wrote, 吾輩は猫である)`、`(東京, capitalOf, 日本)`
- ノードやエッジに **プロパティ（属性）** を持たせる「プロパティグラフ」モデルもあわせて使う（Neo4j 流）
- 代表例: Google Knowledge Graph、Wikidata、DBpedia、Freebase
- 用途: セマンティック検索、推論、推薦、LLM の知識補強（GraphRAG など）

本ツールは「トリプルストア」と「プロパティグラフ」のハイブリッドで、ノード／エッジの双方に任意のキー・バリュー属性を載せられる。

## インストール

```bash
go install github.com/chun37/knowledge-graph/cmd/kg@latest
```

もしくはソースからビルド：

```bash
git clone https://github.com/chun37/knowledge-graph
cd knowledge-graph
go build -o kg ./cmd/kg
```

外部依存はゼロ。Go 1.26+ で確認済み。

## プロジェクト構成

```
.
├── cmd/kg/main.go      CLI エントリポイント
├── graph/
│   ├── graph.go        Graph / Node / Edge と操作
│   └── storage.go      JSON 永続化
└── go.mod
```

`graph` パッケージは単独でライブラリとしても import 可能：

```go
import "github.com/chun37/knowledge-graph/graph"

g := graph.New()
g.AddNode("Alice", []string{"Person"}, map[string]string{"age": "30"})
g.AddEdge("Alice", "knows", "Bob", nil)
```

## データの保存場所

JSON ファイル 1 つに永続化する。

| 環境変数 | 既定値 |
|---|---|
| `KG_DATA` | `~/.kg/data.json` |

書き込みは一時ファイルへ書いてから `rename(2)` で差し替えるので、途中で落ちても元のファイルは壊れない。

## コマンド一覧

### ノード／エッジの追加

| コマンド | 説明 |
|---|---|
| `kg add-node <id> [--label L]... [--prop k=v]...` | ノードを追加または更新。`--label` と `--prop` は複数回指定可。既存ノードに対しては属性をマージ。 |
| `kg add-edge <from> <relation> <to> [--prop k=v]...` | 有向エッジ（トリプル）を追加。端点ノードが存在しなければ自動生成。 |
| `kg add-triple <s> <p> <o>` | `add-edge` のエイリアス。 |

### 削除

| コマンド | 説明 |
|---|---|
| `kg delete-node <id>` | ノードと、それに接続している全エッジを削除。 |
| `kg delete-edge <from> <relation> <to>` | 単一のエッジを削除。 |

### 参照

| コマンド | 説明 |
|---|---|
| `kg show <id>` | ノードのラベル・プロパティ・出入りエッジを表示。 |
| `kg list-nodes [--label L]` | ノード一覧。ラベルでフィルタ可。 |
| `kg list-edges [--relation R]` | エッジ一覧。リレーション名でフィルタ可。 |
| `kg stats` | ノード数・エッジ数・リレーション別カウントを表示。 |

### クエリ

| コマンド | 説明 |
|---|---|
| `kg query [--subject S] [--predicate P] [--object O]` | SPARQL 風のトリプル検索。省略したフラグはワイルドカードとして扱う。 |
| `kg neighbors <id> [--direction out\|in\|both]` | ノードに接続するエッジを表示。既定は `both`。 |
| `kg path <from> <to>` | 2 ノード間の最短パスを無向 BFS で探索。 |

### 入出力

| コマンド | 説明 |
|---|---|
| `kg export [--format json\|triples]` | 標準出力にダンプ。`triples` はタブ区切りの `S\tP\tO` 形式。 |
| `kg help` | ヘルプ表示。 |

## 使用例

```bash
# エンティティの登録
kg add-node Souseki    --label Person --prop birth=1867 --prop country=Japan
kg add-node IAmACat    --label Book   --prop year=1905
kg add-node Mori_Ougai --label Person --prop birth=1862
kg add-node Japan      --label Country

# 関係の登録
kg add-edge Souseki    wrote          IAmACat --prop genre=satire
kg add-edge Souseki    bornIn         Japan
kg add-edge Mori_Ougai bornIn         Japan
kg add-edge Souseki    contemporaryOf Mori_Ougai

# 統計
kg stats
# => nodes=4 edges=4 relations={bornIn=2, contemporaryOf=1, wrote=1}

# ラベルで絞り込み
kg list-nodes --label Person

# あるノードの全貌
kg show Souseki

# トリプル検索 (Souseki が主語のもの全部)
kg query --subject Souseki

# 「日本で生まれた誰か」
kg query --predicate bornIn --object Japan

# 日本に入ってくるエッジ
kg neighbors Japan --direction in

# 最短パス（直接エッジがなければ中継ノード経由を返す）
kg path Souseki Mori_Ougai

# トリプル形式でダンプ
kg export --format triples
```

## データモデル

```go
type Node struct {
    ID         string
    Labels     []string          // "Person", "Book" など複数可
    Properties map[string]string
}

type Edge struct {
    From       string
    Relation   string             // 述語
    To         string
    Properties map[string]string  // エッジ自体の属性 (since, weight など)
}

type Graph struct {
    Nodes map[string]*Node
    Edges []*Edge
}
```

ノード ID は文字列でユニーク。同一 `(from, relation, to)` のエッジは重複追加しても 1 本にマージされ、プロパティだけ上書きされる。
