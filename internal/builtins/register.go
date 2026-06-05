package builtins

import (
	_ "stream-source-tester/internal/input/builtin/mp4"
	_ "stream-source-tester/internal/input/builtin/pcap"
	_ "stream-source-tester/internal/mutation/builtin/dropeverynth"
	_ "stream-source-tester/internal/mutation/builtin/droppackets"
	_ "stream-source-tester/internal/mutation/builtin/identity"
	_ "stream-source-tester/internal/mutation/builtin/reorderfirsttwo"
	_ "stream-source-tester/internal/mutation/builtin/rewritesequence"
	_ "stream-source-tester/internal/mutation/builtin/rewritetimestamp"
	_ "stream-source-tester/internal/mutation/builtin/setmarker"
	_ "stream-source-tester/internal/output/builtin/rtpudp"
	_ "stream-source-tester/internal/output/builtin/rtsp"
)
