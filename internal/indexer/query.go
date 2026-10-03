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

// The indexer's integer filter has only gt, lt and eq: a window (From, To]
// is sent as gt From, lt To+1.
type heightRange struct {
	Gt int64 `json:"gt"`
	Lt int64 `json:"lt"`
}

type pkgPathFilter struct {
	PkgPath struct {
		Eq string `json:"eq"`
	} `json:"pkg_path"`
}

type txFilter struct {
	Success struct {
		Eq bool `json:"eq"`
	} `json:"success"`
	BlockHeight heightRange `json:"block_height"`
	Response    struct {
		Events struct {
			GnoEvent struct {
				Or []pkgPathFilter `json:"_or"`
			} `json:"GnoEvent"`
		} `json:"events"`
	} `json:"response"`
}

type blockFilter struct {
	Height heightRange `json:"height"`
}

type windowVars struct {
	Where  txFilter    `json:"where"`
	Blocks blockFilter `json:"blocks"`
}

type windowData struct {
	LatestBlockHeight int64 `json:"latestBlockHeight"`
	GetTransactions   []struct {
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
	} `json:"getTransactions"`
	GetBlocks []struct {
		Height int64  `json:"height"`
		Time   string `json:"time"`
	} `json:"getBlocks"`
}

type latestData struct {
	LatestBlockHeight int64 `json:"latestBlockHeight"`
}

func newWindowVars(w Window) windowVars {
	var v windowVars
	v.Where.Success.Eq = true
	v.Where.BlockHeight = heightRange{Gt: w.From, Lt: w.To + 1}
	for _, p := range w.Paths {
		var f pkgPathFilter
		f.PkgPath.Eq = p
		v.Where.Response.Events.GnoEvent.Or = append(v.Where.Response.Events.GnoEvent.Or, f)
	}
	v.Blocks.Height = v.Where.BlockHeight
	return v
}
