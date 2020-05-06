import re
from sys import argv

# showBlockList: sorted : 21
reBlkList = re.compile(r'showBlockList:\s*(.*?)\s*:\s+(\d+)\s*$')
lineBlkList = 'showBlockList: unsorted : 21'
assert reBlkList.search(lineBlkList)

# block 0: rot=0 {54.00 91.85 697.92 755.88} col=0 nCols=0 lines=1
reBlock = re.compile(r'block\s+(\d+):+\s+rot=(\d+)\s+\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}.*lines=(\d+)')
lineBlock = 'block 0: rot=0 {54.00 91.85 697.92 755.88} col=0 nCols=0 lines=1'
assert reBlock.search(lineBlock)

# line 0: base=120.24 {42.52 422.51 670.63 694.63} fontSize=24.00 "How people decide what they want to"
# base=18.14 {531.47 566.93 758.55 767.55} fontSize=9.00 "PIONEER"
# line 0: serial=0 base=18.14 {531.47 566.93 758.55 767.55} fontSize=9.00 "PIONEER" col=
reLine = re.compile(r'line\s+(\d+)\s*:\s*serial=\d+\s+base=(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontsize=(\S+)\s*"(.*)"')
lineLine = 'line 0: serial=0 base=18.14 {531.47 566.93 758.55 767.55} fontsize=9.00 "PIONEER" col'
assert reLine.search(lineLine)

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
	return idx, rot, llx, urx, lly, ury, nLines

def parseLine(i, line):
	m = reLine.search(line)
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

def scan(path, wantedTitle):
	print("scan: %s ----------------" % path)
	n = 0
	blocksList = []
	blocks = []
	lines = []
	header = None
	state = 0
	oldState = 0
	blockN = 0
	nBlocks = 0
	with open(path, 'rt', errors='ignore') as f:
		for i1, line in enumerate(f):
			i = i1 + 1
			line = line[:-1].strip()
			if not line:
				continue
			if state == 0:
				m = reBlkList.search(line)
				if m:
					titleLine = line
					title = m.group(1)
					print('title="%s" %s' % (title, path))
					if wantedTitle and title != wantedTitle:
						assert False
						continue
					nBlocks = int(m.group(2))
					blocks = []
					state = 1
					header = line
			elif state == 1:
				idx, rot, llx, urx, lly, ury, nLines = parseBlock(i, line)
				blk = [idx, rot, llx, urx, lly, ury, nLines, line]
				assert idx < nBlocks
				assert idx == len(blocks), (idx, blockN)
				state = 2
				lines = []
				# print('nLines=%d' % nLines)
			elif state == 2:
				idx, base, llx, urx, lly, ury, fontsize, text = parseLine(i, line)
				lines.append((idx, base, llx, urx, lly, ury, fontsize, text, line))
				assert idx < nLines
				if len(lines) == nLines:
					blocks.append((blk, lines))
					if len(blocks) == nBlocks:
						state = 0
						blocksList.append((title, titleLine, blocks))
					else:
						state = 1

			# if state != 0:
			# 	print('state=%d->%d: %s' % (oldState, state, line))
			# 	oldState = state

	assert nBlocks == len(blocks)
	return blocksList

def showLines(header, lines):
	line0 = '%d lines ' % len(lines)
	line0 += 'x' * (80 - len(line0))
	line1 = '+' * 80
	lines = [header, line0] + lines + [line1]
	return '\n'.join(lines)

TOL = 0.1
def equal(x1, x2):
	return abs(x1 - x2) < TOL

def validateBlocks(ttb1, ttb2):
	title1, titleLine1, blocks1 = ttb1
	title2, titleLine2, blocks2 = ttb2
	print('%2d blocks %2d blocks' % (len(blocks1), len(blocks2)))
	msg = "\n\t >>%s<<\n\t >>%s<<" % (titleLine1, titleLine2)
	assert title1 == title2, msg
	# assert len(blocks1) == len(blocks2), msg
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
		msg = '\n\t%s \n- >>%s<<\n\t%s \n- >>%s<<' % (blk1, line1, blk2, line2)
		assert idx1 == idx2, msg
		assert llx1 == llx2, msg
		assert urx1 == urx2, msg
		assert nLines1 == nLines2, msg

		assert len(lines1) == len(lines2)
		m = len(lines1)

		print('    block %2d: %2d entries -----------' % (i, m))

		for j in range(m):
			# print('blk1=%d %s' % (len(blk1[j]), blk1[j]))
			# idx, llx, urx, lly, ury, base, text, line
			idx1, base1, llx1, urx1, lly1, ury1, fontsize1, text1, line1 = lines1[j]
			idx2, base2, llx2, urx2, lly2, ury2, fontsize2, text2, line2 = lines2[j]
			msg = 'j=%d\n\tlines1=%s\n\t>>%s<<\tlines2=%s\n\t>>%s<<' % (j,
				lines1[j][:-1], lines1[j][-1],
				lines2[j][:-1], lines2[j][-1])
			# print('line %2d: %s' % (j, msg))
			assert equal(llx1, llx2), msg
			assert equal(urx1, urx2), msg
			# assert lly1 == lly2, msg
			# assert ury1 == ury2, msg
			assert equal(base1, base2), '%g - %g = %g\n%s' % (base1, base2, base1 - base2, msg)
			assert fontsize1 == fontsize2, msg
			assert text1.replace(' ', '') == text2.replace(' ', ''), msg

			print(i,j,text2)

wantedTitle = None
if len(argv) > 3:
	wantedTitle = argv[3]
	print('wantedTitle="%s"' % wantedTitle)
blockList1 = scan(argv[1], wantedTitle)
blockList2 = scan(argv[2], wantedTitle)
print('blockList1=%d %s' % (len(blockList1), [ttb[0] for ttb in blockList1]))
print('blockList2=%d %s' % (len(blockList2), [ttb[0] for ttb in blockList2]))
assert len(blockList1) == len(blockList2)
for i in range(len(blockList1)):
	ttb1 = blockList1[i]
	ttb2 = blockList2[i]
	print('blocks %d of %d "%s" ========' % (i+1, len(blockList1),  ttb1[0]))
	assert ttb1[0] == ttb2[0]
	validateBlocks(ttb1, ttb2)
