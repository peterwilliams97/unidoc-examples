import re
from sys import argv

# showBlockList: sorted : 21
reBlkList = re.compile(r'showBlockList:\s*(.*):\s+(\d+)\s*$')

lineBlkList = 'showBlockList: sorted : 21'
assert reBlkList.search(lineBlkList)

# block 0: rot=0 {54.00 91.85 697.92 755.88} col=0 nCols=0 lines=1
reBlock = re.compile(r'block\s+(\d+):+\s+rot=(\d+)\s+\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}.*lines=(\d+)')
lineBlock = 'block 0: rot=0 {54.00 91.85 697.92 755.88} col=0 nCols=0 lines=1'
assert reBlock.search(lineBlock)

# line 0: base=120.24 {42.52 422.51 670.63 694.63} fontSize=24.00 "How people decide what they want to"
reLine = re.compile(r'line\s+(\d+)\s*:\s*base=(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontSize=(\S+)\s*"(.*)"')
lineLine = '  line 0: base=120.24 {42.52 422.51 670.63 694.63} fontSize=24.00 "How people decide what they want to"'
assert reLine.search(lineLine)

def parseBlock(line):
	m = reBlock.search(line)
	assert m, '>>>%s<<<' % line
	idx = int(m.group(1))
	rot = int(m.group(2))
	llx = float(m.group(3))
	urx = float(m.group(4))
	lly = float(m.group(5))
	ury = float(m.group(6))
	# base = float(m.group(7))
	nLines = int(m.group(7))
	return idx, rot, llx, urx, lly, ury, nLines

def parseLine(line):
	m = reLine.search(line)
	assert m, '>>>%s<<<' % line
	try:
		idx = int(m.group(1))
		llx = float(m.group(2))
		urx = float(m.group(3))
		lly = float(m.group(4))
		ury = float(m.group(5))
		base = float(m.group(6))
		text = m.group(7)
	except Exception as e:
		print(e, line, m.groups())
		raise
	return idx, llx, urx, lly, ury, base, text


def scan(path):
	print("scan: %s ----------------" % path)
	n = 0
	blocks = []
	lines = []
	header = None
	state = 0
	oldState = 0
	blockN = 0
	with open(path, 'rt') as f:
		for line in f:
			line = line[:-1]
			if not line:
				continue
			if state == 0:
				m = reBlkList.search(line)
				if m:
					titleLine = line
					title = m.group(1)
					nBlocks = int(m.group(2))
					blocks = []
					state = 1
					header = line
			elif state == 1:
				idx, rot, llx, urx, lly, ury, nLines = parseBlock(line)
				blk = [idx, rot, llx, urx, lly, ury, nLines, line]
				assert idx < nBlocks
				assert idx == len(blocks), (idx, blockN)
				state = 2
				lines = []
				# print('nLines=%d' % nLines)
			elif state == 2:
				idx, llx, urx, lly, ury, base, text = parseLine(line) 
				lines.append((idx, llx, urx, lly, ury, base, text, line))
				assert idx < nLines
				if len(lines) == nLines:
					blocks.append((blk, lines))
					if len(blocks) == nBlocks:
						state = 0
					else:
						state = 1
				
			# if state != 0:
			# 	print('state=%d->%d: %s' % (oldState, state, line))	
			# 	oldState = state
			
	assert nBlocks == len(blocks)
	return title, titleLine, blocks


title1, titleLine1, blocks1 = scan(argv[1])
title2, titleLine2, blocks2 = scan(argv[2])
print('%s %d blocks "%s"' % (argv[1], len(blocks1), title1))
print('%s %d blocks "%s"' % (argv[2], len(blocks2), title1))
msg = "\n\t >>%s<<\n\t >>%s<<" % (titleLine1, titleLine2)
assert title1 == title2, msg
assert len(blocks1) == len(blocks2), msg


def showLines(header, lines):
	line0 = '%d lines ' % len(lines)
	line0 += 'x' * (80 - len(line0))
	line1 = '+' * 80
	lines = [header, line0] + lines + [line1]
	return '\n'.join(lines)


TOL = 0.1
def equal(x1, x2):
	return abs(x1 - x2) < TOL

n = min(len(blocks1), len(blocks2))

for i in range(n):
	blk1, lines1 = blocks1[i]
	blk2, lines2 = blocks2[i]
	# print('blk1=%s' % blk1)
	# print('blk2=%s' % blk2)
	# idx, llx, urx, lly, ury, base, text, line
	# blk1=[0, 0, 54.0, 91.85, 697.92, 755.88, 1, 'block 0: rot=0 {54.00 91.85 697.92 755.88} col=0 nCols=0 lines=1']
	# blk2=[0, 0, 54.0, 91.85, 36.0, 114.95, 1, 'block 0: rot=0 {54.00 91.85 36.00 114.95} col=0 nCols=1 lines=1']
	idx1, rot1, llx1, urx1, lly1, ury1, nLines1, line1 = blk1 
	idx2, rot2, llx2, urx2, lly2, ury2, nLines2, line2 = blk2
	msg = '\n\t%s - >>%s<<\n\t%s - >>%s<<' % (blk1, line1, blk2, line2)
	assert idx1 == idx2, msg
	assert llx1 == llx2, msg
	assert urx1 == urx2, msg
	assert nLines1 == nLines2, msg
	
	assert len(lines1) == len(lines2)
	m = len(lines1)

	print('block %2d: %d entries -----------' % (i, m))
	
	for j in range(m):
		# print('blk1=%d %s' % (len(blk1[j]), blk1[j]))
		# idx, llx, urx, lly, ury, base, text, line
		idx1, llx1, urx1, lly1, ury1, base1, text1, line1 = lines1[j] 
		idx2, llx2, urx2, lly2, ury2, base2, text2, line2 = lines2[j] 
		msg = 'j=%d\n\tlines1=%s\n\tlines2=%s' % (j, lines1[j], lines2[j])
		# print('line %2d: %s' % (j, msg))
		assert equal(llx1, llx2), msg
		assert equal(urx1, urx2), msg
		# assert lly1 == lly2, msg
		# assert ury1 == ury2, msg
		assert equal(base1, base2), msg
		assert text1 == text2, msg
		assert 'xxxxx' not in text1, msg
		assert 'xxxxx' not in text2, msg
		# print(i,j,text2)

