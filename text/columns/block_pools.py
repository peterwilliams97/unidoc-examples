import re
from sys import argv

# [INFO]  text_block.go:276 textBlock: before discardDuplicatedText blk=0--------------------------
#  block 0: rot=0 {143.54 468.45 741.93 756.27} col=0 nCols=0 lines=0 pools=1 minBaseIdx=24 maxBaseIdx=24
#  pool 0: baseIdx=24 len=5
#   word 0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"
#   word 1: serial=0 base=99.96 {183.07 271.98 741.93 756.27} fontsize=14.35 "Performance"
#   word 2: serial=0 base=99.96 {277.36 350.01 741.93 756.27} fontsize=14.35 "Document"
#   word 3: serial=0 base=99.96 {355.38 403.87 741.93 756.27} fontsize=14.35 "Layout"
#   word 4: serial=0 base=99.96 {409.23 468.45 741.93 756.27} fontsize=14.35 "Analysis"
# [INFO]  text_block.go:278 ----------xxxx------------

# textBlock: lines built blk=3--------------------------
reBlk0 = re.compile(r'textBlock:\s+(.*)\s+blk=(\d+)')
txtBlk2 = '----------xxxx------------'

lineBlk0 = 'TextOutputDev.cc:1896 textBlock: lines built blk=1--------------------------'
assert reBlk0.search(lineBlk0)

reBlock = re.compile(r'block\s+(\d+):+\s+rot=(\d+)\s+\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}.*lines=(-?\d+)\s+pools=(-?\d+)')
lineBlock = 'block 0: rot=0 {143.54 468.45 741.93 756.27} col=0 nCols=0 lines=7 pools=13 minBaseIdx=- maxBaseIdx=24'
assert reBlock.search(lineBlock)
m = reBlock.search(lineBlock)
nLines = int(m.group(7))
nPools = int(m.group(8))
assert nLines == 7
assert nPools == 13
lineBlock = 'block 0: rot=0 {375.68 417.32 270.00 290.00} col=0 nCols=0 lines=0 pools=1'
m = reBlock.search(lineBlock)
nLines = int(m.group(7))
nPools = int(m.group(8))
assert nLines == 0
assert nPools == 1

rePool = re.compile(r'pool\s+(\d+)\s*:\s*baseIdx=(-?\d+)\s+len=(\d+)')
linePool = 'pool 0: baseIdx=24 len=5'
assert rePool.search(linePool)

reLine = re.compile(r'line\s+(\d+)\s*:\s*serial\s*=\s*\d+\s*base=\s*(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontsize=(\S+)\s*"(.*)"')
lineLine = 'line 0: serial=0 base=99.96 {143.54 468.45 741.93 756.27} fontsize=14.35 "High Performance Document Layout Analysis" col = 0 []'
assert reLine.search(lineLine)

# word 0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"
reWord = re.compile(r'word\s+(\d+)\s*:\s*serial\s*=\s*\d+\s+base=\s*(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontsize\s*=\s*(\S+)\s*"(.*)"')
lineWord = 'word   0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"'
assert reWord.search(lineWord)

def parseBlock(i, line):
	m = reBlock.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	idx = int(m.group(1))
	rot = int(m.group(2))
	llx = float(m.group(3))
	urx = float(m.group(4))
	lly = float(m.group(5))
	ury = float(m.group(6))
	# base = float(m.group(7))
	nLines = int(m.group(7))
	nPools = int(m.group(8))
	return i, idx, rot, llx, urx, lly, ury, nLines, nPools

def parsePool(i, line):
	m = rePool.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	idx = int(m.group(1))
	baseIdx = int(m.group(2))
	nWords = float(m.group(3))
	assert nWords > 0, (i, line)
	return i, idx, baseIdx, nWords, line

def parseLine(i, line):
	m = reLine.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	base = float(m.group(1))
	llx = float(m.group(2))
	urx = float(m.group(3))
	lly = float(m.group(4))
	ury = float(m.group(5))
	fontsize = float(m.group(6))
	text = m.group(7)
	return i, base, llx, urx, lly, ury, fontsize, text, line

def parseWord(i, line):
	'word   0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"'
	m = reWord.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	try:
		idx = int(m.group(1))
		base = float(m.group(2))
		llx = float(m.group(3))
		urx = float(m.group(4))
		lly = float(m.group(5))
		ury = float(m.group(6))
		fontsize = float(m.group(7))
		text = m.group(8)
	except Exception as e:
		print(e, line, m.groups())
		raise
	return idx, base, llx, urx, lly, ury, fontsize, text

def scan(path):
	print('scan: %s -----------------' % path)
	n = 0
	blocks = []
	pools = []
	words = []
	nPools = 0
	nWords = 0
	header = None
	state = 0
	oldState = 0
	with open(path, 'rt', errors='ignore') as f:
		for i1, line in enumerate(f):
			i = i1 + 1
			line = line[:-1].strip()
			if not line:
				continue

			if state == 3 and len(words) == nWords:
				pools.append((pool, words))
				state = 2
				words = []
				nWords = 0
				# print('state=%d->%d  X' % (3, state))
			if state == 2 and len(pools) == nPools:
				blocks.append((header, block,  pools))
				state = 0
				pools = []
				nPools = 0
				# print('state=%d->%d Y' % (2, state))


			if state == 0:
				m = reBlk0.search(line)
				if m:
					header = m.group(1)
					state = 1
			elif state == 1:
				block = list(parseBlock(i, line)) + [line]
				state = 2
				nPools = block[8]
				pools = []
			elif state == 2:
				pool = parsePool(i, line)
				# pools.append(pool)
				state = 3
				nWords = pool[3]
				words = []
			elif state == 3:
				word = parseWord(i, line)
				words.append(word)

			if state != 0:
				# print('state=%d->%d: %2d of %2d pools %2d of %2d words >>%s<<' % (
				# 	oldState, state, len(pools), nPools, len(words), nWords, line))
				assert len(pools) <= nPools
				assert len(words) <= nWords
			oldState = state

	return blocks


def showLines(header, lines):
	line0 = '%d lines ' % len(lines)
	line0 += 'x' * (80 - len(line0))
	line1 = '+' * 80
	lines = [header, line0] + ['>>%s<<' % ln[-1] for ln in lines] + [line1]
	return '\n'.join(lines)


def showPools(header, pools):
	line0 = '%d pools ' % len(pools)
	line0 += 'x' * (80 - len(line0))
	line1 = '+' * 80
	# (idx1, baseIdx1, nWords1, line1), words1
	lines = [header, line0] + ['>>%s<<' % l[0][3] for l in pools] + [line1]
	# lines = [header, line0, line1]
	return '\n'.join(lines)

TOL = 0.1
def equal(x1, x2):
	return abs(x1 - x2) < TOL


blocks1 = scan(argv[1])
blocks2 = scan(argv[2])
print('%s %d blocks' % (argv[1], len(blocks1)))
print('%s %d blocks' % (argv[2], len(blocks2)))
# assert len(blocks1) == len(blocks2)
n = min(len(blocks1), len(blocks2))
for i in range(n):
	header1, _, pools1 = blocks1[i]
	header2, _, pools2 = blocks2[i]
	print('++ block %2d: %2d %2d entries ----------- "%s" "%s"' % (
		i,  len(pools1), len(pools2), header1, header2))
print('=' * 80)

for i in range(n):
	header1, blk1, pools1 = blocks1[i]
	header2, blk2, pools2 = blocks2[i]
	assert header1 == header2, (header1, header2)
	msg =  'i=%d "%s" blk1=%d blk2=%d\npools1=\n%s\npools2=\n%s' % (
		i, header1, len(pools1), len(pools2),
		showPools(header1, pools1),
		showPools(header2, pools2))

	print('block %2d ----------- "%s"\n\t%s\n\t%s' % (i, header1, list(blk1), list(blk2)))
	# print('blk1=%d %s' % (len(blk1), list(blk1)))
	assert len(pools1) == len(pools2), msg
	m = len(pools1)

	ib1, idx1, rot1, llx1, urx1, lly1, ury1, nLines1, nPools1, line1 = blk1
	ib2, idx2, rot2, llx2, urx2, lly2, ury2, nLines2, nPools2, line2 = blk2
	msg = 'i=%d "%s" blk1=%d blk2=%d\nblk1=%s\n>>%s<<\nblk2=%s\n>>%s<<' % (
		i, header1, len(pools1), len(pools2),
		blk1[:-1], blk1[-1],
		blk2[:-1], blk2[-1])
	assert idx1 == idx2, msg
	assert llx1 == llx2, msg
	assert urx1 == urx2, msg
	assert nPools1 == nPools2, msg
	assert nPools1 == len(pools1), msg
	assert nPools2 == len(pools2), msg

	for j in range(m):
		# print('blk1=%d %s' % (len(blk1[j]), blk1[j]))
		(ip1, idx1, baseIdx1, nWords1, line1), words1 = pools1[j]
		(ip2, idx2, baseIdx2, nWords2, line2), words2 = pools2[j]
		msg = 'j=%d\n\tpools1=%s\n\pools2ß=%s' % (j, pools1[j], pools2[j])

		assert idx1 == idx2, msg
		assert baseIdx1 == baseIdx2, msg
		assert nWords1 == nWords2, msg
		assert line1 == line2, msg
		assert nWords1 == len(words1)
		assert nWords2 == len(words2)

print('/' * 80)
