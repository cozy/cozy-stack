package couchdb

// Cursor holds a pointer in a couchdb map reduce results
type Cursor struct {
	Limit     int
	NextKey   interface{}
	NextDocID string
}

// ApplyTo applies the cursor to a ViewRequest
// the transformed ViewRequest will retrive elements from Cursor to
// Limit or StartKey whichever comes first
// Mutates req
func (c *Cursor) ApplyTo(req *ViewRequest) *ViewRequest {
	if c.NextKey != "" {
		if req.Key != nil && req.StartKey == nil {
			req.StartKey = req.Key
			req.EndKey = req.Key
			req.InclusiveEnd = true
			req.Key = nil
		}

		req.StartKey = c.NextKey
		if c.NextDocID != "" {
			req.StartKeyDocID = c.NextDocID
		}
	}

	if c.Limit != 0 {
		req.Limit = c.Limit + 1
	}

	return req
}

// GetNextCursor returns a cursor to the end of a ViewResponse
// it removes the last item from the response to create a Cursor
func GetNextCursor(res *ViewResponse) *Cursor {
	if len(res.Rows) == 0 {
		return &Cursor{}
	}
	next := res.Rows[len(res.Rows)-1]
	res.Rows = res.Rows[:len(res.Rows)-1]

	return &Cursor{
		NextKey:   next.Key,
		NextDocID: next.ID,
	}
}
