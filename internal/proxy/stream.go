package proxy

import (
	"bufio"
	"io"
	"net/http"
)

func streamResponse(w http.ResponseWriter, r *http.Request, resp *http.Response) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, err := io.Copy(w, resp.Body)
		return err
	}

	fw := &flushWriter{w: w, flusher: flusher}
	reader := bufio.NewReader(resp.Body)
	buf := make([]byte, 1024)

	for {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			if _, werr := fw.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}