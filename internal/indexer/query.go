package indexer

const latestQuery = `{ latestBlockHeight }`

const windowQuery = `query Window($where: FilterTransaction!, $blocks: FilterBlock!) {
  latestBlockHeight
  getTransactions(where: $where, order: {heightAndIndex: ASC}) {
    hash block_height index
    response { events { __typename ... on GnoEvent { type pkg_path attrs { key value } } } }
  }
  getBlocks(where: $blocks, order: {height: ASC}) { height time }
}`

const blockTxsQuery = `query BlockTxs($where: FilterTransaction!, $blocks: FilterBlock!) {
  getTransactions(where: $where, order: {heightAndIndex: ASC}) { hash index }
  getBlocks(where: $blocks) { height time }
}`

const txQuery = `query Tx($where: FilterTransaction!) {
  latestBlockHeight
  getTransactions(where: $where) {
    hash block_height index
    response { events { __typename ... on GnoEvent { type pkg_path attrs { key value } } } }
  }
}`

// The indexer's integer filter has only gt, lt and eq: a window (From, To]
// is sent as gt From, lt To+1. A nil bound is left out of the request.
type intFilter struct {
	Eq *int64 `json:"eq,omitempty"`
	Gt *int64 `json:"gt,omitempty"`
	Lt *int64 `json:"lt,omitempty"`
}

func eq(n int64) *intFilter { return &intFilter{Eq: &n} }

type existsFilter struct {
	Exists bool `json:"exists"`
}

// eventsFilter keeps the transactions that carry at least one GnoEvent.
type eventsFilter struct {
	Events struct {
		GnoEvent struct {
			PkgPath existsFilter `json:"pkg_path"`
		} `json:"GnoEvent"`
	} `json:"events"`
}

func withGnoEvent() *eventsFilter {
	var f eventsFilter
	f.Events.GnoEvent.PkgPath.Exists = true
	return &f
}

type txFilter struct {
	Success struct {
		Eq bool `json:"eq"`
	} `json:"success"`
	BlockHeight intFilter     `json:"block_height"`
	Index       *intFilter    `json:"index,omitempty"`
	Response    *eventsFilter `json:"response,omitempty"`
}

type blockFilter struct {
	Height intFilter `json:"height"`
}

// blocksVars serves the window and block queries, which both read block times.
type blocksVars struct {
	Where  txFilter    `json:"where"`
	Blocks blockFilter `json:"blocks"`
}

type txVars struct {
	Where txFilter `json:"where"`
}

type transaction struct {
	Hash        string `json:"hash"`
	BlockHeight int64  `json:"block_height"`
	Index       int    `json:"index"`
	Response    struct {
		Events []struct {
			Typename string `json:"__typename"`
			Type     string `json:"type"`
			PkgPath  string `json:"pkg_path"`
			Attrs    []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"attrs"`
		} `json:"events"`
	} `json:"response"`
}

type block struct {
	Height int64  `json:"height"`
	Time   string `json:"time"`
}

type windowData struct {
	LatestBlockHeight int64         `json:"latestBlockHeight"`
	GetTransactions   []transaction `json:"getTransactions"`
	GetBlocks         []block       `json:"getBlocks"`
}

type txData struct {
	LatestBlockHeight int64         `json:"latestBlockHeight"`
	GetTransactions   []transaction `json:"getTransactions"`
}

type blockTxsData struct {
	GetTransactions []struct {
		Hash  string `json:"hash"`
		Index int    `json:"index"`
	} `json:"getTransactions"`
	GetBlocks []block `json:"getBlocks"`
}

type latestData struct {
	LatestBlockHeight int64 `json:"latestBlockHeight"`
}

func newWindowVars(w Window) blocksVars {
	var v blocksVars
	v.Where.Success.Eq = true
	from, to := w.From, w.To+1
	v.Where.BlockHeight = intFilter{Gt: &from, Lt: &to}
	v.Where.Response = withGnoEvent()
	v.Blocks.Height = v.Where.BlockHeight
	return v
}

func newBlockTxsVars(height int64) blocksVars {
	var v blocksVars
	v.Where.Success.Eq = true
	v.Where.BlockHeight = *eq(height)
	v.Where.Response = withGnoEvent()
	// getBlocks takes the same gt/lt range as the window, which selects one
	// block here.
	from, to := height-1, height+1
	v.Blocks.Height = intFilter{Gt: &from, Lt: &to}
	return v
}

func newTxVars(height int64, index int) txVars {
	var v txVars
	v.Where.Success.Eq = true
	v.Where.BlockHeight = *eq(height)
	v.Where.Index = eq(int64(index))
	return v
}
